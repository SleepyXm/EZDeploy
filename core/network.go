package core

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"
)

const metadataEndpoint = "http://169.254.169.254/latest"

type MetadataClient interface {
	PublicIPv4(context.Context) (string, error)
}
type DNSResolver interface {
	LookupHost(context.Context, string) ([]string, error)
}

type NetworkReport struct {
	Hostname, Record, PublicIPv4 string
	Addresses                    []string
	MetadataAvailable, Match     bool
}

// NetworkDiagnostics compares EC2 IMDSv2 with DNS and never calls a public IP service.
func NetworkDiagnostics(project Project) NetworkReport {
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return CheckNetwork(ctx, project, ec2Metadata{client}, net.DefaultResolver)
}

func CheckNetwork(ctx context.Context, project Project, metadata MetadataClient, resolver DNSResolver) NetworkReport {
	host := strings.Split(strings.TrimSpace(project.Domain), "/")[0]
	report := NetworkReport{Hostname: host, Record: host}
	if strings.HasPrefix(host, "*.") {
		report.Record = host
		report.Hostname = "ezdeploy-check." + strings.TrimPrefix(host, "*.")
	}
	report.PublicIPv4, _ = metadata.PublicIPv4(ctx)
	ip := net.ParseIP(report.PublicIPv4)
	report.MetadataAvailable = ip != nil && ip.To4() != nil
	if !report.MetadataAvailable {
		return report
	}
	addresses, err := resolver.LookupHost(ctx, report.Hostname)
	if err != nil {
		return report
	}
	seen := map[string]bool{}
	for _, address := range addresses {
		if ip := net.ParseIP(address); ip != nil && ip.To4() != nil && !seen[ip.String()] {
			report.Addresses, seen[ip.String()] = append(report.Addresses, ip.String()), true
		}
	}
	sort.Strings(report.Addresses)
	for _, address := range report.Addresses {
		report.Match = report.Match || address == report.PublicIPv4
	}
	return report
}

type ec2Metadata struct{ client *http.Client }

func (metadata ec2Metadata) PublicIPv4(ctx context.Context) (string, error) {
	tokenRequest, _ := http.NewRequestWithContext(ctx, http.MethodPut, metadataEndpoint+"/api/token", nil)
	tokenRequest.Header.Set("X-aws-ec2-metadata-token-ttl-seconds", "60")
	tokenResponse, err := metadata.client.Do(tokenRequest)
	if err != nil {
		return "", err
	}
	defer tokenResponse.Body.Close()
	if tokenResponse.StatusCode != http.StatusOK {
		return "", fmt.Errorf("IMDSv2 token: %s", tokenResponse.Status)
	}
	token, err := io.ReadAll(io.LimitReader(tokenResponse.Body, 4096))
	if err != nil {
		return "", err
	}
	ipRequest, _ := http.NewRequestWithContext(ctx, http.MethodGet, metadataEndpoint+"/meta-data/public-ipv4", nil)
	ipRequest.Header.Set("X-aws-ec2-metadata-token", string(token))
	ipResponse, err := metadata.client.Do(ipRequest)
	if err != nil {
		return "", err
	}
	defer ipResponse.Body.Close()
	if ipResponse.StatusCode != http.StatusOK {
		return "", fmt.Errorf("IMDSv2 IPv4: %s", ipResponse.Status)
	}
	data, err := io.ReadAll(io.LimitReader(ipResponse.Body, 128))
	return strings.TrimSpace(string(data)), err
}
