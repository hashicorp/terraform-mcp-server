package client

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	certPEM = `-----BEGIN CERTIFICATE-----
MIIC/zCCAeegAwIBAgIUSWHe9WcQioaHghrlPYhHyfHtIaUwDQYJKoZIhvcNAQEL
BQAwDzENMAsGA1UEAwwEdGVzdDAeFw0yNTA5MTUyMjMyMzVaFw0yNTA5MTYyMjMy
MzVaMA8xDTALBgNVBAMMBHRlc3QwggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEK
AoIBAQDLmeiweQNGsCQh1aJgia7W4GA1kQooo2MD0Ia0Du+px7pM59DAtWFo4D73
9XXaCMJs+cnvVs/qJGBv1DcR6j970MJtjXqLHH+izMLaOJ1FWHQzZcZ0pKSLu99U
JS+bkdN2osOOSjfADdcW8Bhb6CLHGGPT+8sZTLWIE31oJs6FRx1VgBLjcOwuaFLr
LFS9GjlAhrXG9VYbyG4Im8LchJe+OI2J35trYooV+gSWwHZb6Kle3/9GgHJuw+Kr
puwtQjAPYe1iSey0n4bMcFmTmE41F9pUOYyYCb0zZNREv3nm9TpZh6XZXJnpjW2u
7YQ+EmhEBNqurJ8pvB0fSrbYZvtBAgMBAAGjUzBRMB0GA1UdDgQWBBSJDNiX1BQ9
xrdtp/iIaR1EHL/8EzAfBgNVHSMEGDAWgBSJDNiX1BQ9xrdtp/iIaR1EHL/8EzAP
BgNVHRMBAf8EBTADAQH/MA0GCSqGSIb3DQEBCwUAA4IBAQAVTLylcfx51xF6/ZEC
hCQP+yLIvD+7k6QDa8Sme+rKBSPMZgdYXZ3C21c3/Okb5kLb+R3Mi6rzrktFclOB
Q4CpriOJdt6H5ryLwcCbKL2MQkH3l2/zacYPXFZtUVzfbp9TW0e2SgtrqipOEaAP
kLKyOrY68WRbvlKQq8YvW1+TZXXUPXq0LZQtVWn/N8D81Y7xUABZAouS+H4ZPHKP
YzyNR1mTHw1sRy66lveMCR7YQZ/mBHGuVgKZ7oTQ7NFfGQD/QuOYG4FPrGfbBUCp
aAlrMkrzpu162FcyJfd00B7EovENWpEJ8X4BGWIqNspS5HARlxC7CFP75cITavnB
aJYh
-----END CERTIFICATE-----`

	keyPEM = `-----BEGIN PRIVATE KEY-----
MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQDLmeiweQNGsCQh
1aJgia7W4GA1kQooo2MD0Ia0Du+px7pM59DAtWFo4D739XXaCMJs+cnvVs/qJGBv
1DcR6j970MJtjXqLHH+izMLaOJ1FWHQzZcZ0pKSLu99UJS+bkdN2osOOSjfADdcW
8Bhb6CLHGGPT+8sZTLWIE31oJs6FRx1VgBLjcOwuaFLrLFS9GjlAhrXG9VYbyG4I
m8LchJe+OI2J35trYooV+gSWwHZb6Kle3/9GgHJuw+KrpuwtQjAPYe1iSey0n4bM
cFmTmE41F9pUOYyYCb0zZNREv3nm9TpZh6XZXJnpjW2u7YQ+EmhEBNqurJ8pvB0f
SrbYZvtBAgMBAAECggEAB4ZLkKgW8agascBsP56fCP8iYcsXjHBwvUhEjjIwySjM
mcEwgyAWS/K2XmIikKQQZsgliSmqG03Bsv2talqqrXgF6NgYzmbc0Ga6ZdJqp5CG
Z4w29S0iOALXFQxJxzlbZBqjA7wk9X1ddLzQsvwwOtpL0t65qcNJazsd1aP82nU5
QCxVvQN6nrKCufA97Qt8gw8Trpu9Au5cwedUlCWdLVmbyi6t39Q+NHdh6dOCjcQj
DXKBglm2kCyUmn9PtZBoGNMs0VJJWocm1TuAUDdBZOoUHREhGyRcGP2XmzYS41mV
K0y6GNlM1X8L659CB9+OmUs5p3pbGyWzbcxbSm0M5QKBgQDkfbwE/ZKWb8UjZJuZ
moRi6DB+r0Oj2B+ecuxpVl2o/ljc4DJEYO7g78GPniyuFnPxu36qPOTsnQ2G9d0N
0ZmmgPFSMSc8/RaqDqAEmiv8luJMR3/7t5+f8QeWM/RpHMcFXSTa6YZQVmroFV5E
Xe0IT5F3EmtvDPbRMcZh1q91bQKBgQDkHQxQe87Rq4c6k3jih1j11A0aLKgKAfvd
1YhVM6eyCnRwlxiIxyE9/RHh6Smv2CpoN8BAB0R+JFCSZgriReiJJPauiMn5D9Ur
p4Vj0+u9NJskQIrLrO+vtJl6VP/v+JMSiIDWoCwS+FK8a26A31XP5paSuDxwwLkC
qguKaf38pQKBgDgnFk/7qUzDRyV9kifbq3FLMq/PjsXzVUHxaFwy122ypFAT4Qag
89Pf2CfdbYmTBwd/Vo+ry27C4hIq1hqRSjt7MNNMNSJt6GX+liDLasf2pMKuR6uq
4tXBvGWTexth8R9GVspd3Z5cvoUuyy3uD1SdiVhD9JckDnw2yVQ+GHy5AoGBALAn
YmgQg7DURdRNJ6+/F0qW2xtYWteHmqk/CU3KWviZLDv54l9Vbu1ArXuII8IAKxdZ
3iNTuWezwWicVlFP7PdjTP+Q8G9d858HeXGSqFvu8NM46DXKsTk9PLwEQbRdf61z
LhMPk5l+m92XFLl+PfUTOznH/hyQJw7Qo6LaoOFlAoGAMQb0TSJuGzF5gTqBk/+1
87TBJ9m047jl86llcQnf0Ot8hGcoGBGWOb2jYLI2/0BEBS9i9qbBgD67LmUAJaGr
/BPIy05kfXbgt8UPoPG27aWoCtOFfY57juvQkIBU3ImGOIM+NZHc/ttbtcInFn/Y
nUjEuXHc1fByYMpd2VSTH4M=
-----END PRIVATE KEY-----`
)

func TestTLSConfig(t *testing.T) {
	// Test with no TLS config
	os.Unsetenv("MCP_TLS_CERT_FILE")
	os.Unsetenv("MCP_TLS_KEY_FILE")

	tlsConfig, err := GetTLSConfigFromEnv()
	require.NoError(t, err)
	require.Nil(t, tlsConfig)
}

func TestHTTPServerWithTLS(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok"}`)
	})

	server := httptest.NewTLSServer(mux)
	defer server.Close()

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(server.URL + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestTLSConfigWithValidCert(t *testing.T) {
	// Create temporary cert and key files
	tmpCert, err := os.CreateTemp("", "test_cert_*.pem")
	require.NoError(t, err)
	defer os.Remove(tmpCert.Name())

	tmpKey, err := os.CreateTemp("", "test_key_*.pem")
	require.NoError(t, err)
	defer os.Remove(tmpKey.Name())
	_, err = tmpCert.WriteString(certPEM)
	require.NoError(t, err)
	tmpCert.Close()

	_, err = tmpKey.WriteString(keyPEM)
	require.NoError(t, err)
	tmpKey.Close()

	os.Setenv("MCP_TLS_CERT_FILE", tmpCert.Name())
	os.Setenv("MCP_TLS_KEY_FILE", tmpKey.Name())
	defer func() {
		os.Unsetenv("MCP_TLS_CERT_FILE")
		os.Unsetenv("MCP_TLS_KEY_FILE")
	}()

	tlsConfig, err := GetTLSConfigFromEnv()
	require.NoError(t, err)
	require.NotNil(t, tlsConfig)
	require.Equal(t, tmpCert.Name(), tlsConfig.CertFile)
	require.Equal(t, tmpKey.Name(), tlsConfig.KeyFile)
	require.Equal(t, uint16(tls.VersionTLS12), tlsConfig.Config.MinVersion)
}

func TestTLSConfigValidation(t *testing.T) {
	tests := []struct {
		name      string
		certFile  string
		keyFile   string
		wantNil   bool
		wantError bool
	}{
		{"both empty", "", "", true, false},
		{"cert only", "cert.pem", "", false, true},
		{"key only", "", "key.pem", false, true},
		{"nonexistent files", "nonexistent.pem", "nonexistent.key", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("MCP_TLS_CERT_FILE", tt.certFile)
			os.Setenv("MCP_TLS_KEY_FILE", tt.keyFile)
			defer func() {
				os.Unsetenv("MCP_TLS_CERT_FILE")
				os.Unsetenv("MCP_TLS_KEY_FILE")
			}()

			config, err := GetTLSConfigFromEnv()
			if tt.wantError {
				require.Error(t, err)
				require.Nil(t, config)
			} else if tt.wantNil {
				require.NoError(t, err)
				require.Nil(t, config)
			} else {
				require.NoError(t, err)
				require.NotNil(t, config)
			}
		})
	}
}
