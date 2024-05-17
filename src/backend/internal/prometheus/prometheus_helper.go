package prometheus

import (
	"context"
	"diploma/analytics-exporter/internal/database"
	"fmt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
)

// VisitDuration is a time duration after which visit counts as an end of the session (visit)
const VisitDuration = time.Minute * 30

type AnalyticsStats struct {
	UniqueVisitors  int64
	TotalVisits     int64
	TotalPageViews  int64
	CurrentVisitors int64
	BounceRate      float64

	PagesRate      map[string]int
	SourcesRate    map[string]int
	DevicesRate    map[string]int
	OSsRate        map[string]int
	BrowsersRate   map[string]int
	EntryPagesRate map[string]int
	ExitPagesRate  map[string]int
}

type Visit struct {
	EntryPage             string
	ExitPage              string
	PagesVisited          int
	LastPageViewTimestamp time.Time
}

func GetAnalyticsStats(db database.Database, domain string) (*AnalyticsStats, error) {
	if db == nil {
		return nil, status.Error(codes.InvalidArgument, "database is nil")
	}

	events, err := db.List(context.Background(), domain)

	if err != nil {
		return nil, err
	}

	var pageViewsCount int

	pages := make(map[string]int)
	sources := make(map[string]int)
	devices := make(map[string]int)
	oss := make(map[string]int)
	browsers := make(map[string]int)
	visitsMap := make(map[string][]*Visit)

	var sortedEvents = events.GetEvents()
	if sortedEvents != nil {
		sort.Slice(sortedEvents, func(i, j int) bool {
			return sortedEvents[i].GetTimestamp().AsTime().Before(sortedEvents[j].GetTimestamp().AsTime())
		})
	}

	for _, e := range sortedEvents {

		// count a total of page views
		if e.Type == "pageview" {
			pageViewsCount++
		}

		// extract full url domain and url relative path
		fullUrlDomain, urlPath, err := extractDomainAndPath(e.GetURL())
		if err != nil {
			return nil, err
		}
		// remove the optional .html at the end and write the url relative path to the map
		urlPath = regexp.MustCompile(`\.html$`).ReplaceAllString(urlPath, "")

		// add the url path to the pages statistic
		pages[urlPath]++

		// count a total of visitsMap
		if v, ok := visitsMap[e.GetHashedVisit()]; !ok {
			visitsMap[e.GetHashedVisit()] = make([]*Visit, 1)
			visitsMap[e.GetHashedVisit()][0] = &Visit{
				EntryPage:             urlPath,
				ExitPage:              urlPath,
				PagesVisited:          1,
				LastPageViewTimestamp: e.GetTimestamp().AsTime(),
			}
		} else {
			lastVisit := v[len(v)-1]

			if e.GetTimestamp().AsTime().Sub(lastVisit.LastPageViewTimestamp) > VisitDuration {
				visitsMap[e.GetHashedVisit()] = append(visitsMap[e.GetHashedVisit()], &Visit{
					EntryPage:             urlPath,
					ExitPage:              urlPath,
					PagesVisited:          1,
					LastPageViewTimestamp: e.GetTimestamp().AsTime(),
				})
			} else {
				lastVisit.ExitPage = urlPath
				lastVisit.PagesVisited++
				lastVisit.LastPageViewTimestamp = e.GetTimestamp().AsTime()
			}
		}

		// if referrer is empty that means that the client opened the page directly
		// or HTTP doesn't support this type of referrer
		if e.GetReferrer() == "" {
			sources["Direct/None"]++
		} else {
			var referrerDomain string
			fullReferrerDomain, _, err := extractDomainAndPath(e.GetReferrer())
			if err != nil {
				return nil, err
			}
			referrerDomains := strings.Split(fullReferrerDomain, ".")
			if len(referrerDomains) > 1 {
				slices.Reverse(referrerDomains)
				referrerDomain = fmt.Sprintf("%s.%s", referrerDomains[1], referrerDomains[0])
			} else {
				referrerDomain = fullReferrerDomain
			}

			// if URL and referrer domains aren't the same - that means that the client
			// opened the page from the other website
			if fullUrlDomain != fullReferrerDomain {
				sources[referrerDomain]++
			}
		}

		switch d := e.GetDevice(); {
		case d.GetDesktop():
			devices["Desktop"]++
		case d.GetMobile():
			devices["Mobile"]++
		case d.GetTablet():
			devices["Tablet"]++
		case d.GetBot():
			devices["Bot"]++
		default:
			devices["Unknown"]++
		}

		if os := e.GetOS(); os != "" {
			oss[e.GetOS()]++
		} else {
			oss["Unknown"]++
		}

		if browser := e.GetBrowser(); browser != "" {
			browsers[e.GetBrowser()]++
		} else {
			browsers["Unknown"]++
		}
	}
	entryPages := make(map[string]int)
	exitPages := make(map[string]int)
	var totalVisits int
	var onePageVisits int
	var currentVisitors int

	for _, visits := range visitsMap {
		for _, visit := range visits {
			if visit.PagesVisited == 1 {
				onePageVisits++
			}
			if time.Since(visit.LastPageViewTimestamp).Abs() < 5*time.Minute {
				currentVisitors++
			}
			entryPages[visit.EntryPage]++
			exitPages[visit.ExitPage]++
			totalVisits++
		}
	}

	return &AnalyticsStats{
		UniqueVisitors:  int64(len(visitsMap)),
		TotalVisits:     int64(totalVisits),
		TotalPageViews:  int64(pageViewsCount),
		CurrentVisitors: int64(currentVisitors),
		BounceRate:      float64(onePageVisits) / float64(pageViewsCount),

		PagesRate:      pages,
		SourcesRate:    sources,
		DevicesRate:    devices,
		OSsRate:        oss,
		BrowsersRate:   browsers,
		EntryPagesRate: entryPages,
		ExitPagesRate:  exitPages,
	}, nil
}

func extractDomainAndPath(link string) (string, string, error) {
	regex := regexp.MustCompile(`(?:https?://)?([^/]+)(.*)`)

	matches := regex.FindStringSubmatch(link)
	if len(matches) < 3 {
		return "", "", fmt.Errorf("unable to extract domain and path from the link: %s", link)
	}

	domain := matches[1]
	path := matches[2]

	if path == "" {
		path = "/"
	}

	return domain, path, nil
}
