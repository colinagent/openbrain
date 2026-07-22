package remotecontrol

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Region struct {
	ID          string `json:"regionID"`
	DisplayName string `json:"displayName"`
	Enabled     bool   `json:"enabled"`
	SortOrder   int    `json:"sortOrder"`
}

type CloudEnvironment struct {
	ID                string `json:"environmentID"`
	Name              string `json:"name"`
	RegionID          string `json:"regionID"`
	RoutingGeneration int64  `json:"routingGeneration"`
}

type RemoteClient struct {
	ID            string     `json:"clientID"`
	EnvironmentID string     `json:"environmentID"`
	Name          string     `json:"name"`
	Platform      string     `json:"platform"`
	CreatedAt     time.Time  `json:"createdAt"`
	LastSeenAt    *time.Time `json:"lastSeenAt,omitempty"`
	RevokedAt     *time.Time `json:"revokedAt,omitempty"`
}

type Pairing struct {
	ID        string    `json:"pairingID"`
	Code      string    `json:"code,omitempty"`
	State     string    `json:"state,omitempty"`
	ClientID  string    `json:"clientID,omitempty"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type cloudClient struct {
	root   *url.URL
	client *http.Client
}

type cloudAPIError struct {
	Status            int
	Code              string
	RegionID          string
	RoutingGeneration int64
}

func (e *cloudAPIError) Error() string {
	return fmt.Sprintf("remote-control API %d: %s", e.Status, e.Code)
}

func isTerminalConnectorError(err error) bool {
	var apiError *cloudAPIError
	if !errors.As(err, &apiError) {
		return false
	}
	return apiError.Status == http.StatusUnauthorized ||
		apiError.Code == "not_found" || apiError.Code == "not_authorized" ||
		apiError.Code == "region_changed" || apiError.Code == "region_unavailable"
}

func connectorRegionRedirect(err error) (string, int64, bool) {
	var apiError *cloudAPIError
	if !errors.As(err, &apiError) || apiError.Code != "region_changed" || apiError.RegionID == "" || apiError.RoutingGeneration <= 0 {
		return "", 0, false
	}
	return apiError.RegionID, apiError.RoutingGeneration, true
}

func newCloudClient(rawURL string) (*cloudClient, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(rawURL), "/"))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, errors.New("remote-control API URL must use http or https")
	}
	return &cloudClient{root: parsed, client: &http.Client{Timeout: 15 * time.Second}}, nil
}

func (c *cloudClient) endpoint(path string) string {
	copy := *c.root
	copy.Path = strings.TrimRight(copy.Path, "/") + "/" + strings.TrimLeft(path, "/")
	return copy.String()
}

func regionPath(regionID, suffix string) string {
	return "regions/" + url.PathEscape(regionID) + "/" + strings.TrimLeft(suffix, "/")
}

func (c *cloudClient) regions(ctx context.Context) ([]Region, error) {
	var response struct {
		Regions []Region `json:"regions"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "regions", "", "", nil, &response); err != nil {
		return nil, err
	}
	return response.Regions, nil
}

func (c *cloudClient) enroll(ctx context.Context, sessionToken, regionID string, body any) (CloudEnvironment, string, error) {
	var response struct {
		Environment      CloudEnvironment `json:"environment"`
		ServerCredential string           `json:"serverCredential"`
	}
	err := c.doJSON(ctx, http.MethodPost, regionPath(regionID, "environments/enroll"), sessionToken, "", body, &response)
	return response.Environment, response.ServerCredential, err
}

func (c *cloudClient) switchRegion(ctx context.Context, credential, targetRegionID, environmentID string) (CloudEnvironment, error) {
	var response struct {
		Environment CloudEnvironment `json:"environment"`
	}
	err := c.doJSON(ctx, http.MethodPost, regionPath(targetRegionID, "environments/"+url.PathEscape(environmentID)+"/switch"), credential, "", nil, &response)
	return response.Environment, err
}

func (c *cloudClient) disable(ctx context.Context, sessionToken, regionID, environmentID string) error {
	return c.doJSON(ctx, http.MethodDelete, regionPath(regionID, "environments/"+url.PathEscape(environmentID)), sessionToken, "", nil, nil)
}

func (c *cloudClient) startPairing(ctx context.Context, credential, regionID, environmentID string) (Pairing, error) {
	var pairing Pairing
	err := c.doJSON(ctx, http.MethodPost, regionPath(regionID, "environments/"+url.PathEscape(environmentID)+"/pairings"), credential, "", nil, &pairing)
	return pairing, err
}

func (c *cloudClient) pairingStatus(ctx context.Context, credential, regionID, environmentID, pairingID string) (Pairing, error) {
	var pairing Pairing
	err := c.doJSON(ctx, http.MethodGet, regionPath(regionID, "environments/"+url.PathEscape(environmentID)+"/pairings/"+url.PathEscape(pairingID)), credential, "", nil, &pairing)
	return pairing, err
}

func (c *cloudClient) clients(ctx context.Context, credential, regionID, environmentID string) ([]RemoteClient, error) {
	var response struct {
		Clients []RemoteClient `json:"clients"`
	}
	if err := c.doJSON(ctx, http.MethodGet, regionPath(regionID, "environments/"+url.PathEscape(environmentID)+"/clients"), credential, "", nil, &response); err != nil {
		return nil, err
	}
	return response.Clients, nil
}

func (c *cloudClient) revokeClient(ctx context.Context, sessionToken, regionID, clientID string) error {
	return c.doJSON(ctx, http.MethodDelete, regionPath(regionID, "clients/"+url.PathEscape(clientID)), sessionToken, "", nil, nil)
}

func (c *cloudClient) connectToken(ctx context.Context, credential, regionID string) (string, error) {
	var response struct {
		ConnectToken string `json:"connectToken"`
	}
	if err := c.doJSON(ctx, http.MethodPost, regionPath(regionID, "connect-tokens/server"), credential, "", nil, &response); err != nil {
		return "", err
	}
	return response.ConnectToken, nil
}

func (c *cloudClient) relayURL(regionID string) string {
	parsed := *c.root
	if parsed.Scheme == "https" {
		parsed.Scheme = "wss"
	} else {
		parsed.Scheme = "ws"
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/" + regionPath(regionID, "relay/server")
	return parsed.String()
}

func (c *cloudClient) doJSON(ctx context.Context, method, path, bearer, clientCredential string, input, output any) error {
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.endpoint(path), body)
	if err != nil {
		return err
	}
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	if clientCredential != "" {
		request.Header.Set("X-Remote-Client-Credential", clientCredential)
	}
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var remote struct {
			Code              string `json:"code"`
			RegionID          string `json:"regionID"`
			RoutingGeneration int64  `json:"routingGeneration"`
		}
		_ = json.NewDecoder(io.LimitReader(response.Body, 64*1024)).Decode(&remote)
		if remote.Code == "" {
			remote.Code = http.StatusText(response.StatusCode)
		}
		return &cloudAPIError{Status: response.StatusCode, Code: remote.Code, RegionID: remote.RegionID, RoutingGeneration: remote.RoutingGeneration}
	}
	if output == nil || response.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 256*1024)).Decode(output); err != nil {
		return fmt.Errorf("decode remote-control API response: %w", err)
	}
	return nil
}
