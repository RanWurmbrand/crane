package transfer_pvc

import (
	"strings"
	"testing"
)

func Test_parseSourceDestinationMapping(t *testing.T) {
	tests := []struct {
		name            string
		mapping         string
		wantSource      string
		wantDestination string
		wantErr         bool
	}{
		{
			name:            "given a string with only source name, should return same values for both source and destination",
			mapping:         "validstring",
			wantSource:      "validstring",
			wantDestination: "validstring",
			wantErr:         false,
		},
		{
			name:            "given a string with a valid source to destination mapping, should return correct values for source and destination",
			mapping:         "source:destination",
			wantSource:      "source",
			wantDestination: "destination",
			wantErr:         false,
		},
		{
			name:            "given a string with invalid source to destination mapping, should return error",
			mapping:         "source::destination",
			wantSource:      "",
			wantDestination: "",
			wantErr:         true,
		},
		{
			name:            "given a string with empty destination name, should return error",
			mapping:         "source:",
			wantSource:      "",
			wantDestination: "",
			wantErr:         true,
		},
		{
			name:            "given a mapping with empty source and destination strings, should return error",
			mapping:         ":",
			wantSource:      "",
			wantDestination: "",
			wantErr:         true,
		},
		{
			name:            "given an empty string, should return error",
			mapping:         "",
			wantSource:      "",
			wantDestination: "",
			wantErr:         true,
		},
		{
			name:            "source and destination with spaces",
			mapping:         "source src:destination dst",
			wantSource:      "source src",
			wantDestination: "destination dst",
			wantErr:         false,
		},
		{
			name:            "source and destination with special characters",
			mapping:         "source-src_mysrc:destination-dst_mydest",
			wantSource:      "source-src_mysrc",
			wantDestination: "destination-dst_mydest",
			wantErr:         false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSource, gotDestination, err := parseSourceDestinationMapping(tt.mapping)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseSourceDestinationMapping() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotSource != tt.wantSource {
				t.Errorf("parseSourceDestinationMapping() gotSource = %v, want %v", gotSource, tt.wantSource)
			}
			if gotDestination != tt.wantDestination {
				t.Errorf("parseSourceDestinationMapping() gotDestination = %v, want %v", gotDestination, tt.wantDestination)
			}
		})
	}
}

func Test_endPointsFlagValidate(t *testing.T) {
	tests := []struct {
		name      string
		epType    endpointType
		subdomain string
		ingress   string
		wantErr   bool
	}{
		{
			name:      "nginx-ingress with sub domain",
			epType:    endpointNginx,
			subdomain: "my-test.com",
			ingress:   "nginx",
			wantErr:   false,
		},
		{
			name:      "nginx-ingress with-out sub domain",
			epType:    endpointNginx,
			subdomain: "",
			ingress:   "",
			wantErr:   true,
		},
		{
			name:      "route without subdomain",
			epType:    endpointRoute,
			subdomain: "",
			ingress:   "",
			wantErr:   false,
		},
		{
			name:      "route with subdomain",
			epType:    endpointRoute,
			subdomain: "my-test.com",
			ingress:   "",
			wantErr:   false,
		},
		{
			name:      "empty type with subdomain",
			epType:    "",
			subdomain: "my-test.com",
			ingress:   "",
			wantErr:   false,
		},
		{
			name:      "empty type without subdomain",
			epType:    "",
			subdomain: "",
			ingress:   "",
			wantErr:   true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := EndpointFlags{
				Type:         tc.epType,
				Subdomain:    tc.subdomain,
				IngressClass: tc.ingress,
			}
			err := e.Validate()
			if (err != nil) != tc.wantErr {
				t.Errorf("test missmatched ")
			}
		})

	}

}

func Test_getValidatedResourceName(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectUnchange bool // if true, expect input to be returned unchanged
		expectPrefix   string
	}{
		{
			name:           "short name under 63 chars should return unchanged",
			input:          "my-pvc-name",
			expectUnchange: true,
		},
		{
			name:           "exactly 62 characters should return unchanged",
			input:          strings.Repeat("a", 62),
			expectUnchange: true,
		},
		{
			name:           "exactly 63 characters should trigger hash",
			input:          strings.Repeat("a", 63),
			expectUnchange: false,
			expectPrefix:   "crane-",
		},
		{
			name:           "long name 100 characters should return hashed version",
			input:          strings.Repeat("b", 100),
			expectUnchange: false,
			expectPrefix:   "crane-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getValidatedResourceName(tt.input)

			if tt.expectUnchange {
				if got != tt.input {
					t.Errorf("getValidatedResourceName() = %v, want %v", got, tt.input)
				}
			} else {
				if !strings.HasPrefix(got, tt.expectPrefix) {
					t.Errorf("getValidatedResourceName() = %v, expected to start with %v", got, tt.expectPrefix)
				}
				if len(got) > 63 {
					t.Errorf("getValidatedResourceName() returned %d chars, expected <= 63", len(got))
				}
			}
		})
	}

	// Test determinism: same input should produce same output
	t.Run("same long name should produce same hash", func(t *testing.T) {
		longName := strings.Repeat("c", 100)
		result1 := getValidatedResourceName(longName)
		result2 := getValidatedResourceName(longName)
		if result1 != result2 {
			t.Errorf("getValidatedResourceName() not deterministic: got %v and %v", result1, result2)
		}
	})

	// Test uniqueness: different inputs should produce different outputs
	t.Run("different long names should produce different hashes", func(t *testing.T) {
		longName1 := strings.Repeat("d", 100)
		longName2 := strings.Repeat("e", 100)
		result1 := getValidatedResourceName(longName1)
		result2 := getValidatedResourceName(longName2)
		if result1 == result2 {
			t.Errorf("getValidatedResourceName() produced same hash for different inputs: %v", result1)
		}
	})
}

func Test_PvcFlags_Validate(t *testing.T) {
	tests := []struct {
		name             string
		sourceName       string
		destName         string
		sourceNamespace  string
		destNamespace    string
		wantErr          bool
		wantErrContains  string
	}{
		{
			name:            "all fields set should return nil",
			sourceName:      "my-pvc",
			destName:        "my-pvc-dest",
			sourceNamespace: "source-ns",
			destNamespace:   "dest-ns",
			wantErr:         false,
		},
		{
			name:            "missing source name should return error",
			sourceName:      "",
			destName:        "my-pvc-dest",
			sourceNamespace: "source-ns",
			destNamespace:   "dest-ns",
			wantErr:         true,
			wantErrContains: "source pvc name",
		},
		{
			name:            "missing destination name should return error",
			sourceName:      "my-pvc",
			destName:        "",
			sourceNamespace: "source-ns",
			destNamespace:   "dest-ns",
			wantErr:         true,
			wantErrContains: "destnation pvc name",
		},
		{
			name:            "missing source namespace should return error",
			sourceName:      "my-pvc",
			destName:        "my-pvc-dest",
			sourceNamespace: "",
			destNamespace:   "dest-ns",
			wantErr:         true,
			wantErrContains: "source pvc namespace",
		},
		{
			name:            "missing destination namespace should return error",
			sourceName:      "my-pvc",
			destName:        "my-pvc-dest",
			sourceNamespace: "source-ns",
			destNamespace:   "",
			wantErr:         true,
			wantErrContains: "destination pvc namespace",
		},
		{
			name:            "only names set no namespaces should return source namespace error first",
			sourceName:      "my-pvc",
			destName:        "my-pvc-dest",
			sourceNamespace: "",
			destNamespace:   "",
			wantErr:         true,
			wantErrContains: "source pvc namespace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &PvcFlags{
				Name: mappedNameVar{
					source:      tt.sourceName,
					destination: tt.destName,
				},
				Namespace: mappedNameVar{
					source:      tt.sourceNamespace,
					destination: tt.destNamespace,
				},
			}

			err := p.Validate()

			if (err != nil) != tt.wantErr {
				t.Errorf("PvcFlags.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.wantErrContains != "" {
				if !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Errorf("PvcFlags.Validate() error = %v, should contain %v", err, tt.wantErrContains)
				}
			}
		})
	}
}
