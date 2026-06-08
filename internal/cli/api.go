package cli

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/pendig/rute-bayar/internal/config"
	"github.com/pendig/rute-bayar/internal/domain"
	"github.com/pendig/rute-bayar/internal/provider/ipaymu"
	"github.com/pendig/rute-bayar/internal/storage/sqlite"
)

const apiRequestTimeoutDefault = 30 * time.Second

type pathTemplateOperation struct {
	method string
	path   string
}

func apiCommand(ctx context.Context, stdout, stderr io.Writer, args []string) error {
	if len(args) == 0 {
		printAPIRootUsage(stdout)
		return nil
	}

	if args[0] == "-h" || args[0] == "--help" {
		printAPIRootUsage(stdout)
		return nil
	}

	providerCode, err := parseProvider(args[0])
	if err != nil {
		return err
	}

	if len(args) == 1 {
		printAPIProviderUsage(stdout, providerCode)
		return nil
	}

	if args[1] == "-h" || args[1] == "--help" {
		printAPIProviderUsage(stdout, providerCode)
		return nil
	}

	return apiProviderCommand(ctx, stdout, stderr, providerCode, args[1:])
}

func apiProviderCommand(ctx context.Context, stdout, stderr io.Writer, providerCode domain.ProviderCode, args []string) error {
	cfg := config.Load()
	fs := flag.NewFlagSet(fmt.Sprintf("api %s", providerCode), flag.ContinueOnError)
	fs.SetOutput(stderr)

	method := fs.String("method", "GET", "HTTP method, e.g. GET/POST/PATCH/PUT")
	pathArg := fs.String("path", "", "API path (for example /v2/charge)")
	baseURL := fs.String("base-url", "", "override provider API base URL")
	operation := fs.String("operation", "", "optional operation shortcut")
	environment := fs.String("environment", cfg.Environment, "provider environment: sandbox or production")
	dbPath := fs.String("db", cfg.DBPath, "sqlite database path")
	queryFlags := &stringMapFlag{}
	fs.Var(queryFlags, "query", "query parameters (repeatable, key=value)")
	headerFlags := &stringSliceMapFlag{}
	fs.Var(headerFlags, "header", "request headers (repeatable, key=value)")
	body := fs.String("body", "", "raw request body")
	bodyFile := fs.String("body-file", "", "read request body from file")
	skipAuth := fs.Bool("skip-auth", false, "do not load credentials from provider onboarding")
	pathParams := &stringMapFlag{}
	fs.Var(pathParams, "path-param", "replace operation/path placeholder (repeatable, key=value)")
	showCurl := fs.Bool("show-curl", false, "print curl-equivalent command before executing")
	timeout := fs.Duration("timeout", apiRequestTimeoutDefault, "request timeout, e.g. 30s")

	if err := fs.Parse(args); err != nil {
		return err
	}

	methodValue := strings.ToUpper(strings.TrimSpace(*method))
	if methodValue == "" {
		return fmt.Errorf("--method is required")
	}

	environmentValue := strings.TrimSpace(*environment)
	if err := validateEnvironment(environmentValue); err != nil {
		return err
	}

	resolvedPath := strings.TrimSpace(*pathArg)
	if resolvedPath == "" {
		if strings.TrimSpace(*operation) != "" {
			if candidate, ok := resolveAPIOperation(providerCode, *operation); ok {
				resolvedPath = candidate.path
				if strings.EqualFold(*method, "GET") && candidate.method != "" {
					methodValue = candidate.method
				}
			} else {
				return fmt.Errorf("operation %q is not available for %q", *operation, providerCode)
			}
		} else {
			return fmt.Errorf("either --operation or --path is required")
		}
	}

	pathParamValues := copyStringMap(pathParams.values)
	var unresolved []string
	for _, match := range regexp.MustCompile(`\{([^/{}]+)\}`).FindAllStringSubmatch(resolvedPath, -1) {
		if len(match) != 2 {
			continue
		}
		placeholder := match[1]
		if value, ok := pathParamValues[placeholder]; ok {
			resolvedPath = strings.ReplaceAll(resolvedPath, "{"+placeholder+"}", url.PathEscape(strings.TrimSpace(value)))
		} else {
			unresolved = append(unresolved, placeholder)
		}
	}
	if len(unresolved) > 0 {
		return fmt.Errorf("path has unresolved parameter%s: %s", pluralS(len(unresolved)), strings.Join(uniqueStrings(unresolved), ", "))
	}

	var rawBody []byte
	if strings.TrimSpace(*bodyFile) != "" {
		if strings.TrimSpace(*body) != "" {
			return fmt.Errorf("cannot set both --body and --body-file")
		}
		b, err := os.ReadFile(*bodyFile)
		if err != nil {
			return fmt.Errorf("read body-file: %w", err)
		}
		rawBody = b
	} else {
		rawBody = []byte(*body)
	}

	requestTimeout := *timeout
	if requestTimeout <= 0 {
		requestTimeout = apiRequestTimeoutDefault
	}

	serviceBaseURL := strings.TrimSpace(*baseURL)
	if serviceBaseURL == "" {
		serviceBaseURL = apiDefaultBaseURL(providerCode, domain.Environment(environmentValue))
	}
	serviceBaseURL = strings.TrimRight(serviceBaseURL, "/")

	endpoint, err := url.Parse(serviceBaseURL)
	if err != nil {
		return fmt.Errorf("invalid base-url %q: %w", serviceBaseURL, err)
	}
	requestURL, err := url.Parse(serviceBaseURL + resolvedPath)
	if err != nil {
		return fmt.Errorf("invalid path %q: %w", resolvedPath, err)
	}
	if requestURL.IsAbs() {
		endpoint = requestURL
	} else {
		endpoint = endpoint.ResolveReference(requestURL)
	}

	query := endpoint.Query()
	for key, value := range copyStringMap(queryFlags.values) {
		query.Set(key, value)
	}
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, methodValue, endpoint.String(), strings.NewReader(string(rawBody)))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	if len(rawBody) > 0 && !strings.EqualFold(methodValue, http.MethodGet) && !strings.EqualFold(methodValue, http.MethodHead) {
		request.Header.Set("Content-Type", "application/json")
	}

	if !*skipAuth {
		store, err := sqlite.Open(ctx, *dbPath)
		if err != nil {
			return err
		}
		defer store.Close()

		account, err := store.GetProviderAccount(ctx, providerCode, domain.Environment(environmentValue))
		if err != nil {
			return err
		}
		if err := applyStoredCredentials(providerCode, request, account.CredentialJSON, rawBody); err != nil {
			return err
		}
	}

	for key, values := range copyStringSliceMap(headerFlags.values) {
		for _, value := range values {
			request.Header.Add(key, value)
		}
	}

	if *showCurl {
		printAPICurl(stdout, methodValue, request.URL.String(), request.Header, rawBody)
	}

	client := &http.Client{Timeout: requestTimeout}
	resp, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("call provider api: %w", err)
	}
	defer resp.Body.Close()

	fmt.Fprintf(stdout, "HTTP/%d %s\n", resp.ProtoMajor, resp.Status)
	fmt.Fprintln(stdout, "content-type:", resp.Header.Get("Content-Type"))
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}
	if len(respBody) == 0 {
		fmt.Fprintln(stdout, "empty body")
		return nil
	}
	if maybeJSON := prettyPrintJSON(respBody); maybeJSON != "" {
		fmt.Fprintln(stdout, maybeJSON)
		return nil
	}
	fmt.Fprintln(stdout, string(respBody))
	return nil
}

func printAPIRootUsage(w io.Writer) {
	fmt.Fprintln(w, "API raw mode")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  rutebayar api <provider> [--operation|--path ...]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Providers:")
	for _, provider := range domain.SupportedProviders() {
		fmt.Fprintf(w, "  %s\n", provider)
	}
}

func printAPIProviderUsage(w io.Writer, providerCode domain.ProviderCode) {
	fmt.Fprintf(w, "Rutebayar API mode: %s\n\n", providerCode)
	fmt.Fprintf(w, "Usage:\n  rutebayar api %s --method GET --path /...\n", providerCode)
	fmt.Fprintln(w, "\nCommon options:")
	fmt.Fprintln(w, "  --base-url\toverride API base URL")
	fmt.Fprintln(w, "  --method\tHTTP method")
	fmt.Fprintln(w, "  --path\tAPI path relative to base URL")
	fmt.Fprintln(w, "  --operation\tshortcut to common API operation")
	fmt.Fprintln(w, "  --query\tquery params (repeatable key=value)")
	fmt.Fprintln(w, "  --header\theader override (repeatable key=value)")
	fmt.Fprintln(w, "  --body, --body-file\trequest body")
	fmt.Fprintln(w, "  --skip-auth\tdo not load credentials from provider account")
	fmt.Fprintln(w, "\nExamples:")
	for _, line := range apiProviderUsageExamples(providerCode) {
		fmt.Fprintf(w, "  %s\n", line)
	}
	fmt.Fprintln(w)
}

func apiProviderUsageExamples(providerCode domain.ProviderCode) []string {
	switch providerCode {
	case domain.ProviderMidtrans:
		return []string{
			"rutebayar api midtrans --operation auth-test",
			"rutebayar api midtrans --operation charge --method POST --body '{\"transaction_details\":{...}}'",
			"rutebayar api midtrans --operation snap-transaction --method POST --body '{\"transaction_details\":{...}}'",
			"rutebayar api midtrans --operation status --path-param order_id=RB-001",
			"rutebayar api midtrans --operation check-status --path-param order_id=RB-001",
			"rutebayar api midtrans --path /v2/<order_id>/status --path-param order_id=RB-001",
			"rutebayar api midtrans --operation approve --path-param order_id=RB-001 --method POST",
			"rutebayar api midtrans --operation deny --path-param order_id=RB-001 --method POST",
			"rutebayar api midtrans --operation cancel --path-param order_id=RB-001 --method POST",
			"rutebayar api midtrans --operation expire --path-param order_id=RB-001 --method POST",
			"rutebayar api midtrans --operation refund --path-param order_id=RB-001 --method POST --body '{\"refund_key\":\"rfnd\",\"amount\":1000}'",
		}
	case domain.ProviderXendit:
		return []string{
			"rutebayar api xendit --operation auth-balance",
			"rutebayar api xendit --method GET --path /sessions/<session_id> --path-param session_id=...",
			"rutebayar api xendit --operation session-create --method POST --body '{...}'",
		}
	case domain.ProviderDoku:
		return []string{
			"rutebayar api doku --operation checkout --method POST --body '{...}'",
			"rutebayar api doku --method GET --path /orders/v1/status/<invoice> --path-param invoice=...",
		}
	case domain.ProviderIPaymu:
		return []string{
			"rutebayar api ipaymu --path /api/v2/payment-channels --method GET",
			"rutebayar api ipaymu --path /api/v2/transaction --method POST --body '{...}'",
		}
	default:
		return []string{"rutebayar api <provider> --method GET --path /"}
	}
}

var manualMidtransAPIOperationAliases = map[string]pathTemplateOperation{
	"auth-test":           {method: "GET", path: "/v2/rute-bayar-auth-test/status"},
	"auth":                {method: "GET", path: "/v2/rute-bayar-auth-test/status"},
	"ping":                {method: "GET", path: "/v2/rute-bayar-auth-test/status"},
	"snap":                {method: "POST", path: "/snap/v1/transactions"},
	"snap-transaction":    {method: "POST", path: "/snap/v1/transactions"},
	"snap-v1":             {method: "POST", path: "/snap/v1/transactions"},
	"status":              {method: "GET", path: "/v2/{order_id}/status"},
	"check-status":        {method: "GET", path: "/v2/{order_id}/status"},
	"approve":             {method: "POST", path: "/v2/{order_id}/approve"},
	"approve-transaction": {method: "POST", path: "/v2/{order_id}/approve"},
	"challenge-approve":   {method: "POST", path: "/v2/{order_id}/approve"},
	"deny":                {method: "POST", path: "/v2/{order_id}/deny"},
	"challenge-deny":      {method: "POST", path: "/v2/{order_id}/deny"},
	"fraud-deny":          {method: "POST", path: "/v2/{order_id}/deny"},
	"cancel":              {method: "POST", path: "/v2/{order_id}/cancel"},
	"cancel-transaction":  {method: "POST", path: "/v2/{order_id}/cancel"},
	"expire":              {method: "POST", path: "/v2/{order_id}/expire"},
	"expire-transaction":  {method: "POST", path: "/v2/{order_id}/expire"},
	"charge":              {method: "POST", path: "/v2/charge"},
	"create":              {method: "POST", path: "/v2/charge"},
	"refund":              {method: "POST", path: "/v2/{order_id}/refund"},
	"refund-transaction":  {method: "POST", path: "/v2/{order_id}/refund"},
	"card-token":          {method: "GET", path: "/v2/card/register"},
	"card-register":       {method: "GET", path: "/v2/card/register"},
	"token":               {method: "GET", path: "/v2/token"},
	"client-token":        {method: "GET", path: "/v2/token"},
	"payment-token":       {method: "GET", path: "/v2/token"},
}

func resolveAPIOperation(providerCode domain.ProviderCode, operation string) (pathTemplateOperation, bool) {
	key := strings.TrimSpace(strings.ToLower(operation))
	switch providerCode {
	case domain.ProviderMidtrans:
		if candidate, ok := generatedMidtransAPIOperationAliases[key]; ok {
			return candidate, true
		}
		if candidate, ok := manualMidtransAPIOperationAliases[key]; ok {
			return candidate, true
		}
	case domain.ProviderXendit:
		switch key {
		case "auth-balance", "balance":
			return pathTemplateOperation{method: "GET", path: "/balance"}, true
		case "session-create", "create":
			return pathTemplateOperation{method: "POST", path: "/sessions"}, true
		case "session-status", "status":
			return pathTemplateOperation{method: "GET", path: "/sessions/{session_id}"}, true
		}
	case domain.ProviderDoku:
		switch key {
		case "checkout":
			return pathTemplateOperation{method: "POST", path: "/checkout/v1/payment"}, true
		case "order-status", "status":
			return pathTemplateOperation{method: "GET", path: "/orders/v1/status/{invoice_number_or_request_id}"}, true
		}
	case domain.ProviderIPaymu:
		switch key {
		case "payment-channels", "channels":
			return pathTemplateOperation{method: "GET", path: "/api/v2/payment-channels"}, true
		case "transaction":
			return pathTemplateOperation{method: "POST", path: "/api/v2/transaction"}, true
		}
	}
	return pathTemplateOperation{}, false
}

func apiDefaultBaseURL(providerCode domain.ProviderCode, env domain.Environment) string {
	switch providerCode {
	case domain.ProviderMidtrans:
		if env == domain.EnvironmentProduction {
			return "https://api.midtrans.com"
		}
		return "https://api.sandbox.midtrans.com"
	case domain.ProviderXendit:
		return "https://api.xendit.co"
	case domain.ProviderDoku:
		if env == domain.EnvironmentProduction {
			return "https://api.doku.com"
		}
		return "https://api-sandbox.doku.com"
	case domain.ProviderIPaymu:
		if env == domain.EnvironmentProduction {
			return "https://my.ipaymu.com"
		}
		return "https://sandbox.ipaymu.com"
	default:
		return ""
	}
}

func applyStoredCredentials(providerCode domain.ProviderCode, req *http.Request, credentialJSON []byte, rawBody []byte) error {
	switch providerCode {
	case domain.ProviderMidtrans:
		var credential struct {
			ServerKey string `json:"server_key"`
		}
		if err := json.Unmarshal(credentialJSON, &credential); err != nil {
			return fmt.Errorf("read midtrans credential: %w", err)
		}
		serverKey := strings.TrimSpace(credential.ServerKey)
		if serverKey == "" {
			return fmt.Errorf("midtrans server key is not configured")
		}
		req.SetBasicAuth(serverKey, "")
	case domain.ProviderXendit:
		var credential struct {
			SecretKey string `json:"secret_key"`
		}
		if err := json.Unmarshal(credentialJSON, &credential); err != nil {
			return fmt.Errorf("read xendit credential: %w", err)
		}
		secretKey := strings.TrimSpace(credential.SecretKey)
		if secretKey == "" {
			return fmt.Errorf("xendit secret key is not configured")
		}
		req.SetBasicAuth(secretKey, "")
	case domain.ProviderDoku:
		var credential struct {
			ClientID  string `json:"client_id"`
			SecretKey string `json:"secret_key"`
		}
		if err := json.Unmarshal(credentialJSON, &credential); err != nil {
			return fmt.Errorf("read doku credential: %w", err)
		}
		clientID := strings.TrimSpace(credential.ClientID)
		if clientID == "" {
			return fmt.Errorf("doku client id is not configured")
		}
		secretKey := strings.TrimSpace(credential.SecretKey)
		if secretKey == "" {
			return fmt.Errorf("doku secret key is not configured")
		}
		req.Header.Set("Client-Id", clientID)
		if err := applyDokuSignature(req, rawBody, secretKey); err != nil {
			return err
		}
	case domain.ProviderIPaymu:
		var credential struct {
			VA      string `json:"va"`
			APIKey  string `json:"api_key"`
			Account string `json:"account"`
		}
		if err := json.Unmarshal(credentialJSON, &credential); err != nil {
			return fmt.Errorf("read ipaymu credential: %w", err)
		}
		va := strings.TrimSpace(credential.VA)
		apiKey := strings.TrimSpace(credential.APIKey)
		if va == "" || apiKey == "" {
			return fmt.Errorf("ipaymu credentials are not configured")
		}
		req.Header.Set("va", va)
		req.Header.Set("account", strings.TrimSpace(credential.Account))
		timestamp := time.Now().UTC().Format("20060102150405")
		req.Header.Set("timestamp", timestamp)

		var signaturePayload any
		if len(rawBody) > 0 {
			var decoded any
			if err := json.Unmarshal(rawBody, &decoded); err == nil {
				signaturePayload = decoded
			} else {
				signaturePayload = string(rawBody)
			}
		}
		signature := ipaymu.GenerateSignature(req.Method, va, apiKey, timestamp, signaturePayload)
		req.Header.Set("signature", signature)
	default:
		return fmt.Errorf("provider %q is not supported in api mode", providerCode)
	}
	return nil
}

func applyDokuSignature(req *http.Request, rawBody []byte, secretKey string) error {
	requestID := dokuRequestID()
	requestTimestamp := time.Now().UTC().Format(time.RFC3339)
	targetPath := strings.TrimSpace(req.URL.Path)
	if targetPath == "" {
		targetPath = "/"
	}

	clientID := strings.TrimSpace(req.Header.Get("Client-Id"))
	if clientID == "" {
		return fmt.Errorf("doku client id is not configured")
	}
	req.Header.Set("Request-Id", requestID)
	req.Header.Set("Request-Timestamp", requestTimestamp)

	digest, signature := dokuGenerateSignature(clientID, requestID, requestTimestamp, targetPath, rawBody, secretKey)
	if digest != "" {
		req.Header.Set("Digest", digest)
	}
	req.Header.Set("Signature", signature)
	return nil
}

func dokuGenerateSignature(clientID, requestID, requestTimestamp, requestTarget string, body []byte, secretKey string) (digest string, signature string) {
	if trimmed := strings.TrimSpace(requestTarget); trimmed != "" {
		requestTarget = ensureLeadingSlash(trimmed)
	}
	component := strings.Builder{}
	component.WriteString("Client-Id:" + clientID)
	component.WriteString("\nRequest-Id:" + requestID)
	component.WriteString("\nRequest-Timestamp:" + requestTimestamp)
	component.WriteString("\nRequest-Target:" + requestTarget)
	if len(body) > 0 {
		digest = dokuDigest(body)
		component.WriteString("\nDigest:" + digest)
	}

	mac := hmac.New(sha256.New, []byte(secretKey))
	_, _ = io.WriteString(mac, component.String())
	signature = "HMACSHA256=" + base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return
}

func dokuRequestID() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Sprintf("rutebayar-%d", time.Now().UTC().UnixNano())
	}
	return hex.EncodeToString(raw)
}

func dokuDigest(body []byte) string {
	sum := sha256.Sum256(body)
	return base64.StdEncoding.EncodeToString(sum[:])
}

func ensureLeadingSlash(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "/") {
		return value
	}
	return "/" + value
}

func printAPICurl(w io.Writer, method, rawURL string, headers http.Header, body []byte) {
	parts := []string{"curl", "-i", "-X", method, fmt.Sprintf("%q", rawURL)}
	for key, values := range headers {
		for _, value := range values {
			parts = append(parts, "-H", fmt.Sprintf("%q", fmt.Sprintf("%s: %s", key, value)))
		}
	}
	if len(body) > 0 {
		parts = append(parts, "--data", fmt.Sprintf("%q", string(body)))
	}
	fmt.Fprintln(w, strings.Join(parts, " "))
}

func copyStringSliceMap(values map[string][]string) map[string][]string {
	if len(values) == 0 {
		return map[string][]string{}
	}
	copied := make(map[string][]string, len(values))
	for key, list := range values {
		copied[key] = append([]string(nil), list...)
	}
	return copied
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func pluralS(count int) string {
	if count > 1 {
		return "s"
	}
	return ""
}

func prettyPrintJSON(raw []byte) string {
	var parsed any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ""
	}
	pretty, err := json.MarshalIndent(parsed, "", "  ")
	if err != nil {
		return ""
	}
	return string(pretty)
}
