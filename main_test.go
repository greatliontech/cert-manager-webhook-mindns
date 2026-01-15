package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

func TestMindnsSolver_Name(t *testing.T) {
	solver := &mindnsSolver{}
	assert.Equal(t, "mindns", solver.Name())
}

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name    string
		json    *extapi.JSON
		want    mindnsConfig
		wantErr bool
	}{
		{
			name: "nil config",
			json: nil,
			want: mindnsConfig{},
		},
		{
			name: "empty config",
			json: &extapi.JSON{Raw: []byte(`{}`)},
			want: mindnsConfig{},
		},
		{
			name: "full config",
			json: &extapi.JSON{Raw: []byte(`{"serverAddr":"mindns.default.svc:50051","zone":"example.com."}`)},
			want: mindnsConfig{
				ServerAddr: "mindns.default.svc:50051",
				Zone:       "example.com.",
			},
		},
		{
			name: "server only",
			json: &extapi.JSON{Raw: []byte(`{"serverAddr":"localhost:50051"}`)},
			want: mindnsConfig{
				ServerAddr: "localhost:50051",
			},
		},
		{
			name:    "invalid json",
			json:    &extapi.JSON{Raw: []byte(`{invalid}`)},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := loadConfig(tt.json)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExtractZone(t *testing.T) {
	tests := []struct {
		name         string
		resolvedZone string
		want         string
	}{
		{
			name:         "with trailing dot",
			resolvedZone: "example.com.",
			want:         "example.com.",
		},
		{
			name:         "without trailing dot",
			resolvedZone: "example.com",
			want:         "example.com.",
		},
		{
			name:         "subdomain with dot",
			resolvedZone: "sub.example.com.",
			want:         "sub.example.com.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractZone(tt.resolvedZone)
			assert.Equal(t, tt.want, got)
		})
	}
}
