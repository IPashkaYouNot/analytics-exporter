package analytics

import (
	"context"
	"crypto/rand"
	"diploma/analytics-exporter/pkg/api/analytics"
	"encoding/hex"
	"fmt"
	"github.com/google/uuid"
	"github.com/mileusna/useragent"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"io"
	"time"
)

var (
	DailySalt          []byte
	DailySaltTimestamp time.Time
)

const DailySaltLifetime = time.Hour * 24
const DailySaltBytesAmount = 32

// CreateEvent creates event in the database as *catalog.Customer
func (s *analyticsServer) CreateEvent(ctx context.Context, r *analytics.Event) (*emptypb.Empty, error) {
	if r == nil {
		return nil, status.Error(codes.InvalidArgument, "request is nil")
	}
	if r.GetDomain() == "" {
		return nil, status.Error(codes.InvalidArgument, "domain is missing")
	}
	md, _ := metadata.FromIncomingContext(ctx)
	for key, val := range md {
		fmt.Printf("Key: %s, value: %s\n", key, val)
	}

	fmt.Println(DailySaltTimestamp)
	fmt.Println(time.Now())
	if DailySalt == nil || time.Since(DailySaltTimestamp) > DailySaltLifetime {
		DailySalt = make([]byte, DailySaltBytesAmount)
		_, err := io.ReadFull(rand.Reader, DailySalt)
		if err != nil {
			return nil, fmt.Errorf("error while creating the daily salt: %w", err)
		}
		DailySaltTimestamp = time.Now()
		fmt.Println("Generated a new daily salt")
	}

	// Generate new ID
	id := uuid.New().String()

	// Get the hash of the visit by formula: hash(daily_salt + website_domain + ip_address + user_agent)
	s.h.Write(DailySalt)
	s.h.Write([]byte(r.GetDomain()))
	s.h.Write([]byte(md["x-forwarded-for"][0]))
	s.h.Write([]byte(md["grpcgateway-user-agent"][0]))
	visitHashValue := s.h.Sum(nil)
	visitEncodedHashString := hex.EncodeToString(visitHashValue)

	defer func() {
		s.h.Reset()
	}()

	// Construct a new Protobuf wrapped timestamp from the current time.
	timePbNow := timestamppb.Now()

	// Parse the user agent header
	ua := useragent.Parse(md["grpcgateway-user-agent"][0])

	var device analytics.Device
	switch {
	case ua.Mobile:
		device = analytics.Device{
			Device: &analytics.Device_Mobile{
				Mobile: true,
			},
		}
	case ua.Tablet:
		device = analytics.Device{
			Device: &analytics.Device_Tablet{
				Tablet: true,
			},
		}
	case ua.Desktop:
		device = analytics.Device{
			Device: &analytics.Device_Desktop{
				Desktop: true,
			},
		}
	case ua.Bot:
		device = analytics.Device{
			Device: &analytics.Device_Bot{
				Bot: true,
			},
		}
	}

	e := &analytics.Event{
		ID:          id,
		Type:        r.GetType(),
		URL:         r.GetURL(),
		Domain:      r.GetDomain(),
		Referrer:    r.GetReferrer(),
		Browser:     ua.Name,
		OS:          ua.OS,
		Device:      &device,
		HashedVisit: visitEncodedHashString,
		Meta:        r.GetMeta(),
		Props:       r.GetProps(),
		Timestamp:   timePbNow,
	}

	fmt.Println(e)

	if err := s.db.Insert(ctx, e); err != nil {
		return nil, status.Errorf(codes.Internal, "cannot create event %s: %v", r.GetDomain(), e)
	}
	return &emptypb.Empty{}, nil
}

// ListEvents returns events slice from the database as *analytics.Events
func (s *analyticsServer) ListEvents(ctx context.Context, r *wrapperspb.StringValue) (*analytics.Events, error) {
	entries, err := s.db.List(ctx, r.String())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "cannot list customers: %v", err)
	}
	return entries, nil
}
