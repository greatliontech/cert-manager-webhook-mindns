package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/client-go/rest"

	"github.com/cert-manager/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	"github.com/cert-manager/cert-manager/pkg/acme/webhook/cmd"
	"github.com/greatliontech/mindns/pkg/mindnspb"
)

// bearerAuth implements grpc.PerRPCCredentials for bearer token authentication.
type bearerAuth struct {
	token string
}

func (b bearerAuth) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{
		"authorization": "Bearer " + b.token,
	}, nil
}

func (b bearerAuth) RequireTransportSecurity() bool {
	return false
}

var GroupName = os.Getenv("GROUP_NAME")

func main() {
	if GroupName == "" {
		panic("GROUP_NAME must be specified")
	}

	cmd.RunWebhookServer(GroupName, &mindnsSolver{})
}

// mindnsSolver implements the cert-manager DNS01 solver interface
// using mindns as the DNS provider.
type mindnsSolver struct {
	mu         sync.RWMutex
	client     mindnspb.MindnsServiceClient
	conn       *grpc.ClientConn
	serverAddr string
	token      string
}

// mindnsConfig is the configuration for the mindns solver.
// Users configure this in their Issuer/ClusterIssuer webhook config.
type mindnsConfig struct {
	// ServerAddr is the address of the mindns gRPC server (e.g., "mindns.default.svc:50051")
	ServerAddr string `json:"serverAddr"`

	// Zone is the DNS zone to manage (e.g., "example.com.")
	// If not specified, it will be derived from the challenge domain.
	Zone string `json:"zone,omitempty"`

	// Token is the optional bearer token for authentication.
	// If not specified, falls back to MINDNS_TOKEN environment variable.
	Token string `json:"token,omitempty"`
}

// Name returns the solver name used in Issuer configurations.
func (s *mindnsSolver) Name() string {
	return "mindns"
}

// Present creates a TXT record for the ACME DNS01 challenge.
func (s *mindnsSolver) Present(ch *v1alpha1.ChallengeRequest) error {
	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	client, err := s.getClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to connect to mindns: %w", err)
	}

	// Determine the zone
	zone := cfg.Zone
	if zone == "" {
		zone = extractZone(ch.ResolvedZone)
	}

	// The challenge record name (e.g., "_acme-challenge.www.example.com.")
	recordName := ch.ResolvedFQDN

	// First, try to get existing TXT records to append
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	getReq := &mindnspb.GetRRsetRequest{}
	getReq.SetZone(zone)
	getReq.SetName(recordName)
	getReq.SetType(mindnspb.Type_TypeTXT)

	existing, err := client.GetRRset(ctx, getReq)

	var records []*mindnspb.TXT
	if err == nil && existing != nil {
		// Parse existing records
		for _, any := range existing.GetRecords() {
			prr, err := mindnspb.ProtoRRFromAny(any)
			if err != nil {
				continue
			}
			if existingTxt, ok := prr.(*mindnspb.TXT); ok {
				// Check if this key already exists
				if existingTxt.GetData() != nil {
					if slices.Contains(existingTxt.GetData().GetTxt(), ch.Key) {
						// Already present, nothing to do
						return nil
					}
				}
				records = append(records, existingTxt)
			}
		}
	}

	// Create new TXT record with the challenge key
	txt := &mindnspb.TXT{}
	hdr := &mindnspb.RR_Header{}
	hdr.SetName(recordName)
	hdr.SetTtl(60)
	txt.SetHdr(hdr)
	data := &mindnspb.TXTData{}
	data.SetTxt([]string{ch.Key})
	txt.SetData(data)

	// Add our new record
	records = append(records, txt)

	// Build RRset
	rrset := &mindnspb.RRset{}
	rrset.SetName(recordName)
	rrset.SetType(mindnspb.Type_TypeTXT)
	rrset.SetTtl(60)

	// Convert to Any
	anyRecords, err := mindnspb.ProtoRRsToAny(toProtoRRSlice(records))
	if err != nil {
		return fmt.Errorf("failed to convert records: %w", err)
	}
	rrset.SetRecords(anyRecords)

	// Set the RRset
	ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel2()

	setReq := &mindnspb.SetRRsetRequest{}
	setReq.SetZone(zone)
	setReq.SetRrset(rrset)

	_, err = client.SetRRset(ctx2, setReq)
	if err != nil {
		return fmt.Errorf("failed to set TXT record: %w", err)
	}

	return nil
}

// CleanUp removes the TXT record for the ACME DNS01 challenge.
func (s *mindnsSolver) CleanUp(ch *v1alpha1.ChallengeRequest) error {
	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	client, err := s.getClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to connect to mindns: %w", err)
	}

	zone := cfg.Zone
	if zone == "" {
		zone = extractZone(ch.ResolvedZone)
	}

	recordName := ch.ResolvedFQDN

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get existing TXT records
	getReq := &mindnspb.GetRRsetRequest{}
	getReq.SetZone(zone)
	getReq.SetName(recordName)
	getReq.SetType(mindnspb.Type_TypeTXT)

	existing, err := client.GetRRset(ctx, getReq)
	if err != nil {
		// Record doesn't exist, nothing to clean up
		return nil
	}

	// Filter out the record with our key
	var remaining []*mindnspb.TXT
	for _, any := range existing.GetRecords() {
		prr, err := mindnspb.ProtoRRFromAny(any)
		if err != nil {
			continue
		}
		if txt, ok := prr.(*mindnspb.TXT); ok {
			// Check if this is our record
			isOurs := false
			if txt.GetData() != nil {
				for _, t := range txt.GetData().GetTxt() {
					if t == ch.Key {
						isOurs = true
						break
					}
				}
			}
			if !isOurs {
				remaining = append(remaining, txt)
			}
		}
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel2()

	if len(remaining) == 0 {
		// No records left, delete the RRset
		delReq := &mindnspb.DeleteRRsetRequest{}
		delReq.SetZone(zone)
		delReq.SetName(recordName)
		delReq.SetType(mindnspb.Type_TypeTXT)

		_, err = client.DeleteRRset(ctx2, delReq)
		if err != nil {
			return fmt.Errorf("failed to delete TXT record: %w", err)
		}
	} else {
		// Update with remaining records
		rrset := &mindnspb.RRset{}
		rrset.SetName(recordName)
		rrset.SetType(mindnspb.Type_TypeTXT)
		rrset.SetTtl(60)

		anyRecords, err := mindnspb.ProtoRRsToAny(toProtoRRSlice(remaining))
		if err != nil {
			return fmt.Errorf("failed to convert records: %w", err)
		}
		rrset.SetRecords(anyRecords)

		setReq := &mindnspb.SetRRsetRequest{}
		setReq.SetZone(zone)
		setReq.SetRrset(rrset)

		_, err = client.SetRRset(ctx2, setReq)
		if err != nil {
			return fmt.Errorf("failed to update TXT records: %w", err)
		}
	}

	return nil
}

// Initialize is called when the webhook starts.
func (s *mindnsSolver) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	// Connection is established lazily in getClient
	return nil
}

// getClient returns a gRPC client for the given server address and optional token.
func (s *mindnsSolver) getClient(cfg mindnsConfig) (mindnspb.MindnsServiceClient, error) {
	// Resolve token: config takes precedence over env var
	token := cfg.Token
	if token == "" {
		token = os.Getenv("MINDNS_TOKEN")
	}

	s.mu.RLock()
	if s.client != nil && s.conn != nil && s.serverAddr == cfg.ServerAddr && s.token == token {
		s.mu.RUnlock()
		return s.client, nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	// Double-check after acquiring write lock
	if s.client != nil && s.conn != nil && s.serverAddr == cfg.ServerAddr && s.token == token {
		return s.client, nil
	}

	// Close existing connection if any
	if s.conn != nil {
		s.conn.Close()
	}

	dialOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}

	// Add bearer auth if token is configured
	if token != "" {
		dialOpts = append(dialOpts, grpc.WithPerRPCCredentials(bearerAuth{token: token}))
	}

	conn, err := grpc.NewClient(cfg.ServerAddr, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", cfg.ServerAddr, err)
	}

	s.conn = conn
	s.serverAddr = cfg.ServerAddr
	s.token = token
	s.client = mindnspb.NewMindnsServiceClient(conn)

	return s.client, nil
}

// loadConfig decodes the webhook configuration.
func loadConfig(cfgJSON *extapi.JSON) (mindnsConfig, error) {
	cfg := mindnsConfig{}
	if cfgJSON == nil {
		return cfg, nil
	}
	if err := json.Unmarshal(cfgJSON.Raw, &cfg); err != nil {
		return cfg, fmt.Errorf("error decoding solver config: %w", err)
	}
	return cfg, nil
}

// extractZone extracts the zone from the resolved zone string.
// cert-manager provides this as "example.com." format.
func extractZone(resolvedZone string) string {
	// Ensure it ends with a dot
	if !strings.HasSuffix(resolvedZone, ".") {
		return resolvedZone + "."
	}
	return resolvedZone
}

// toProtoRRSlice converts []*mindnspb.TXT to []mindnspb.ProtoRR
func toProtoRRSlice(txts []*mindnspb.TXT) []mindnspb.ProtoRR {
	result := make([]mindnspb.ProtoRR, len(txts))
	for i, txt := range txts {
		result[i] = txt
	}
	return result
}
