package main

import (
	"context"
	"os"
	"testing"
	"time"

	acmetest "github.com/cert-manager/cert-manager/test/acme"
	"github.com/greatliontech/mindns/pkg/testutil"
)

const testZone = "example.com."

func TestConformance(t *testing.T) {
	if os.Getenv("CONFORMANCE_TEST") != "1" {
		t.Skip("Skipping conformance test. Set CONFORMANCE_TEST=1 to run.")
	}

	ctx := context.Background()

	// Use fixed ports so testdata/mindns-solver/config.json can be committed
	const (
		dnsAddr  = "127.0.0.1:19853"
		grpcAddr = "127.0.0.1:19854"
	)

	// Create test server with in-memory storage
	server, err := testutil.NewTestServer(testutil.Config{
		DNSAddr:  dnsAddr,
		GRPCAddr: grpcAddr,
	})
	if err != nil {
		t.Fatalf("failed to create test server: %v", err)
	}
	defer server.Stop(ctx)

	// Start servers in background
	server.StartBackground()
	time.Sleep(100 * time.Millisecond)

	t.Logf("Started mindns test server: gRPC=%s DNS=%s", grpcAddr, dnsAddr)

	// Create test zone
	if err := server.CreateZone(ctx, testZone); err != nil {
		t.Fatalf("failed to create zone: %v", err)
	}

	// Create solver
	solver := &mindnsSolver{}


	// Run conformance test
	fixture := acmetest.NewFixture(solver,
		acmetest.SetResolvedZone(testZone),
		acmetest.SetManifestPath("testdata/mindns-solver"),
		acmetest.SetDNSServer(server.DNSAddr()),
		acmetest.SetUseAuthoritative(false),
		acmetest.SetStrict(true),
	)
	// need to uncomment and  RunConformance delete runBasic and runExtended once https://github.com/cert-manager/cert-manager/pull/4835 is merged
	// fixture.RunConformance(t)
	fixture.RunBasic(t)
	fixture.RunExtended(t)
}
