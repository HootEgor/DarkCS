package services

import (
	"DarkCS/entity"
	"DarkCS/internal/config"
	"DarkCS/internal/lib/sl"
	"DarkCS/internal/lib/util"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"
)

type ZohoService struct {
	clientID            string
	clientSecret        string
	defaultRefreshToken string
	refreshToken        string
	refreshUrl          string
	crmUrl              string
	scope               string
	apiVersion          string
	tokenExpiresIn      time.Time
	log                 *slog.Logger

	// tokenMu guards refreshToken (the current access token, despite the name), crmUrl
	// and tokenExpiresIn: every request reads them and a refresh rewrites them.
	tokenMu sync.RWMutex
	client  *http.Client
}

// zohoHTTPTimeout bounds every Zoho call so a stalled connection can't hang a bot step.
const zohoHTTPTimeout = 30 * time.Second

func NewZohoService(conf *config.Config, log *slog.Logger) *ZohoService {

	return &ZohoService{
		clientID:            conf.Zoho.ClientId,
		clientSecret:        conf.Zoho.ClientSecret,
		defaultRefreshToken: conf.Zoho.RefreshToken,
		refreshToken:        conf.Zoho.RefreshToken,
		refreshUrl:          conf.Zoho.RefreshUrl,
		crmUrl:              conf.Zoho.CrmUrl,
		scope:               conf.Zoho.Scope,
		apiVersion:          conf.Zoho.ApiVersion,
		log:                 log.With(sl.Module("zoho")),
		client:              &http.Client{Timeout: zohoHTTPTimeout},
	}
}

// token returns the current access token.
func (s *ZohoService) token() string {
	s.tokenMu.RLock()
	defer s.tokenMu.RUnlock()
	return s.refreshToken
}

// tokenExpiring reports whether the access token is within 5 minutes of expiry.
func (s *ZohoService) tokenExpiring() bool {
	s.tokenMu.RLock()
	defer s.tokenMu.RUnlock()
	return !time.Now().Before(s.tokenExpiresIn.Add(-5 * time.Minute))
}

// apiDomain returns the CRM base URL, which a token refresh may update.
func (s *ZohoService) apiDomain() string {
	s.tokenMu.RLock()
	defer s.tokenMu.RUnlock()
	return s.crmUrl
}

// refreshTokenCall refreshes the access token unless another goroutine already did.
// Callers check expiry without the lock, so many can arrive together after expiry;
// the double-check under the lock turns that into one refresh instead of a burst that
// Zoho rate-limits.
func (s *ZohoService) refreshTokenCall() error {
	s.tokenMu.Lock()
	defer s.tokenMu.Unlock()
	if time.Now().Before(s.tokenExpiresIn.Add(-5 * time.Minute)) {
		return nil
	}
	return s.doRefreshLocked()
}

// forceRefresh refreshes regardless of expiry (after a 401). tokenUsed is the token that
// was rejected: if it was already replaced meanwhile, no new refresh is made.
func (s *ZohoService) forceRefresh(tokenUsed string) error {
	s.tokenMu.Lock()
	defer s.tokenMu.Unlock()
	if s.refreshToken != tokenUsed {
		return nil
	}
	return s.doRefreshLocked()
}

// do sends an authorized request. On 401 it refreshes the token once and retries, which
// covers tokens revoked or expired before the local expiry estimate.
func (s *ZohoService) do(req *http.Request) (*http.Response, error) {
	tokenUsed := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
	resp, err := s.client.Do(req)
	if err != nil || resp.StatusCode != http.StatusUnauthorized || (req.Body != nil && req.GetBody == nil) {
		return resp, err
	}
	_ = resp.Body.Close()

	if err = s.forceRefresh(tokenUsed); err != nil {
		return nil, fmt.Errorf("refresh after 401: %w", err)
	}
	retry := req.Clone(req.Context())
	if req.GetBody != nil {
		if retry.Body, err = req.GetBody(); err != nil {
			return nil, err
		}
	}
	retry.Header.Set("Authorization", "Bearer "+s.token())
	return s.client.Do(retry)
}

// doRefreshLocked exchanges the refresh token for a new access token. Caller holds tokenMu.
func (s *ZohoService) doRefreshLocked() error {
	form := url.Values{}
	form.Add("client_id", s.clientID)
	form.Add("client_secret", s.clientSecret)
	form.Add("refresh_token", s.defaultRefreshToken)
	form.Add("grant_type", "refresh_token")

	resp, err := s.client.PostForm(s.refreshUrl, form)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("refresh token failed: %s", string(bodyBytes))
	}

	var response entity.TokenResponse
	if err = json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if response.AccessToken != "" {
		s.refreshToken = response.AccessToken
	}
	if response.ApiDomain != "" {
		s.crmUrl = response.ApiDomain
	}

	// Zoho access tokens live one hour; use expires_in when given.
	lifetime := time.Hour
	if response.ExpiresIn > 0 {
		lifetime = time.Duration(response.ExpiresIn) * time.Second
	}
	s.tokenExpiresIn = time.Now().Add(lifetime)

	if response != (entity.TokenResponse{}) {
		// Never log the response itself: it contains the access token.
		s.log.Debug("refresh token succeeded", slog.Int("expires_in", response.ExpiresIn))
	} else {
		return fmt.Errorf("empty response from Zoho API")
	}

	return nil
}

func (s *ZohoService) createContact(contactData entity.Contact) (string, error) {

	email, err := util.ValidateEmail(contactData.Email)
	if err != nil {
		s.log.With(
			sl.Err(err),
		).Debug("invalid email")
	}

	contactData.Email = email

	if contactData.Email == "" && contactData.Phone == "" {
		return "", fmt.Errorf("email and phone are empty")
	}

	// Prepare request body
	payload := map[string]interface{}{
		"data": []entity.Contact{contactData},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}

	fullURL, err := buildURL(s.apiDomain(), s.scope, s.apiVersion, "Contacts")
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest(
		http.MethodPost,
		fullURL,
		bytes.NewBuffer(body),
	)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.token())
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.do(req)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	s.log.With(
		slog.String("response", string(bodyBytes)),
	).Debug("create contact response")

	var apiResp entity.ZohoAPIResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if len(apiResp.Data) == 0 {
		return "", fmt.Errorf("empty response data")
	}

	item := apiResp.Data[0]

	// Handle DUPLICATE_DATA gracefully
	if item.Status == "error" {
		if item.Code == "DUPLICATE_DATA" {
			var dup entity.DuplicateDetails
			if err := json.Unmarshal(item.Details, &dup); err != nil {
				return "", fmt.Errorf("failed to parse duplicate details: %w", err)
			}

			var generic map[string]interface{}
			if err := json.Unmarshal(item.Details, &generic); err != nil {
				return "", fmt.Errorf("failed to parse details: %w", err)
			}
			s.log.With(
				slog.String("duplicate_id", dup.DuplicateRecord.ID),
				//slog.Any("owner", dup.DuplicateRecord.Owner),
				slog.String("module", dup.DuplicateRecord.Module.APIName),
				slog.Any("Details", generic),
			).Debug("duplicate record detected")
			return dup.DuplicateRecord.ID, nil
		}

		if item.Code == "MULTIPLE_OR_MULTI_ERRORS" {
			var multiErr entity.MultipleErrors
			if err := json.Unmarshal(item.Details, &multiErr); err != nil {
				return "", fmt.Errorf("failed to parse multiple errors: %w", err)
			}
			if len(multiErr.Errors) == 0 {
				return "", fmt.Errorf("zoho error [%s] without details: %s", item.Code, item.Message)
			}
			s.log.With(
				slog.Any("error_message", multiErr.Errors[0].Message),
			).Debug("multiple errors detected")
			return multiErr.Errors[0].Details.DuplicateRecord.ID, nil
		}
		return "", fmt.Errorf("zoho error [%s]: %s", item.Code, item.Message)
	}

	// Success path: extract the record ID
	var successDetails entity.SuccessContactDetails
	if err := json.Unmarshal(item.Details, &successDetails); err != nil {
		return "", fmt.Errorf("failed to parse success ID: %w", err)
	}

	return successDetails.ID, nil

}

func (s *ZohoService) createOrder(orderData entity.ZohoOrder) error {
	// Prepare payload
	payload := map[string]interface{}{
		"data": []entity.ZohoOrder{orderData},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	fullURL, err := buildURL(s.apiDomain(), s.scope, s.apiVersion, "Sales_Orders")
	if err != nil {
		return err
	}

	req, err := http.NewRequest(
		http.MethodPost,
		fullURL,
		bytes.NewBuffer(body),
	)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", "Bearer "+s.token())
	req.Header.Set("Content-Type", "application/json")

	// Payload size only: the order body holds the customer's name, phone, email and address.
	log := s.log.With(
		slog.String("url", fullURL),
		slog.String("method", req.Method),
		slog.Int("payload_bytes", len(body)))
	t := time.Now()
	defer func() {
		log = log.With(slog.Duration("duration", time.Since(t)))
		if err != nil {
			log.Error("create order", sl.Err(err))
		} else {
			log.Debug("create order")
		}
	}()

	// Execute request
	resp, err := s.do(req)
	if err != nil {
		s.log.With(
			sl.Err(err),
		).Debug("response")
		return err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	s.log.With(
		slog.String("response", string(bodyBytes)),
	).Debug("create order response")

	var apiResp entity.ZohoAPIResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if len(apiResp.Data) == 0 {
		return fmt.Errorf("empty response data")
	}

	item := apiResp.Data[0]

	if item.Status != "success" {
		// Decode error details
		var errDetails entity.ErrorDetails
		_ = json.Unmarshal(item.Details, &errDetails)

		return fmt.Errorf(
			"order not created: [%s] %s (field: %s, path: %s)",
			item.Code,
			item.Message,
			errDetails.APIName,
			errDetails.JSONPath,
		)
	}

	// Decode success
	var success entity.SuccessOrderDetails
	if err := json.Unmarshal(item.Details, &success); err != nil {
		return fmt.Errorf("failed to parse order ID: %w", err)
	}

	s.log.With(
		slog.Any("order response", success),
	).Debug("order created successfully")

	return nil

}

func (s *ZohoService) updateOrder(orderData entity.ZohoOrder, id string) error {
	// Prepare payload
	payload := map[string]interface{}{
		"data": []entity.ZohoOrder{orderData},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	fullURL, err := buildURL(s.apiDomain(), s.scope, s.apiVersion, "Sales_Orders", id)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(
		http.MethodPut,
		fullURL,
		bytes.NewBuffer(body),
	)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", "Bearer "+s.token())
	req.Header.Set("Content-Type", "application/json")

	// Execute request
	resp, err := s.do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	s.log.With(
		slog.String("response", string(bodyBytes)),
	).Debug("create order response")

	var apiResp entity.ZohoAPIResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if len(apiResp.Data) == 0 {
		return fmt.Errorf("empty response data")
	}

	item := apiResp.Data[0]

	if item.Status != "success" {
		// Decode error details
		var errDetails entity.ErrorDetails
		_ = json.Unmarshal(item.Details, &errDetails)

		return fmt.Errorf(
			"order not created: [%s] %s (field: %s, path: %s)",
			item.Code,
			item.Message,
			errDetails.APIName,
			errDetails.JSONPath,
		)
	}

	// Decode success
	var success entity.SuccessOrderDetails
	if err := json.Unmarshal(item.Details, &success); err != nil {
		return fmt.Errorf("failed to parse order ID: %w", err)
	}

	s.log.With(
		slog.Any("order response", success),
	).Debug("order created successfully")

	return nil

}

func buildURL(base string, paths ...string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("invalid base URL: %w", err)
	}

	// Join additional path segments cleanly
	allPaths := append([]string{u.Path}, paths...)
	u.Path = path.Join(allPaths...)

	return u.String(), nil
}
