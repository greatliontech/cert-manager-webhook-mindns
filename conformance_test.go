package main

import (
	"context"
	"os"
	"testing"
	"time"

	acmetest "github.com/cert-manager/cert-manager/test/acme"
	"github.com/cert-manager/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	"github.com/cert-manager/cert-manager/test/apiserver"
	"github.com/greatliontech/mindns/pkg/testutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/client-go/kubernetes"
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

// TestTokenSecretRef tests that the solver can fetch tokens from K8s Secrets
func TestTokenSecretRef(t *testing.T) {
	if os.Getenv("CONFORMANCE_TEST") != "1" {
		t.Skip("Skipping conformance test. Set CONFORMANCE_TEST=1 to run.")
	}

	ctx := context.Background()

	// Use different ports than conformance test
	const (
		dnsAddr  = "127.0.0.1:19855"
		grpcAddr = "127.0.0.1:19856"
	)

	// Start envtest for K8s API
	env, stopFn := apiserver.RunBareControlPlane(t)
	defer stopFn()

	// Create K8s client
	k8sClient, err := kubernetes.NewForConfig(env.Config)
	if err != nil {
		t.Fatalf("failed to create k8s client: %v", err)
	}

	// Create test namespace
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-token-secret",
		},
	}
	if _, err := k8sClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{}); err != nil {
		t.Fatalf("failed to create namespace: %v", err)
	}

	// Create secret with token
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mindns-token",
			Namespace: ns.Name,
		},
		Data: map[string][]byte{
			"token": []byte("test-bearer-token"),
		},
	}
	if _, err := k8sClient.CoreV1().Secrets(ns.Name).Create(ctx, secret, metav1.CreateOptions{}); err != nil {
		t.Fatalf("failed to create secret: %v", err)
	}

	// Create test server
	server, err := testutil.NewTestServer(testutil.Config{
		DNSAddr:  dnsAddr,
		GRPCAddr: grpcAddr,
	})
	if err != nil {
		t.Fatalf("failed to create test server: %v", err)
	}
	defer server.Stop(ctx)

	server.StartBackground()
	time.Sleep(100 * time.Millisecond)

	// Create test zone
	if err := server.CreateZone(ctx, testZone); err != nil {
		t.Fatalf("failed to create zone: %v", err)
	}

	// Create solver and initialize with K8s client
	solver := &mindnsSolver{}
	if err := solver.Initialize(env.Config, nil); err != nil {
		t.Fatalf("failed to initialize solver: %v", err)
	}

	// Create challenge request with tokenSecretRef
	configJSON := `{
		"serverAddr": "127.0.0.1:19856",
		"tokenSecretRef": {
			"name": "mindns-token",
			"key": "token"
		}
	}`
	ch := &v1alpha1.ChallengeRequest{
		DNSName:           "example.com",
		Key:               "token-test-key",
		ResourceNamespace: ns.Name,
		ResolvedFQDN:      "_acme-challenge.example.com.",
		ResolvedZone:      testZone,
		Config:            &extapi.JSON{Raw: []byte(configJSON)},
	}

	// Test Present - this will fetch token from secret
	t.Log("Testing Present with tokenSecretRef...")
	if err := solver.Present(ch); err != nil {
		t.Fatalf("Present failed: %v", err)
	}
	t.Log("Present succeeded - token was fetched from K8s Secret")

	// Test CleanUp
	if err := solver.CleanUp(ch); err != nil {
		t.Fatalf("CleanUp failed: %v", err)
	}
	t.Log("CleanUp succeeded")

	// Test with missing secret - should fail
	t.Log("Testing Present with missing secret...")
	chMissing := &v1alpha1.ChallengeRequest{
		DNSName:           "example.com",
		Key:               "missing-secret-test",
		ResourceNamespace: ns.Name,
		ResolvedFQDN:      "_acme-challenge.example.com.",
		ResolvedZone:      testZone,
		Config:            &extapi.JSON{Raw: []byte(`{"serverAddr": "127.0.0.1:19856", "tokenSecretRef": {"name": "nonexistent"}}`)},
	}
	if err := solver.Present(chMissing); err == nil {
		t.Fatal("Present should have failed with missing secret")
	} else {
		t.Logf("Present correctly failed with missing secret: %v", err)
	}
}
