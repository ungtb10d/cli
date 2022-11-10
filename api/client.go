package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/ungtb10d/cli/v2/internal/ghinstance"
	"github.com/cli/go-gh"
	ghAPI "github.com/cli/go-gh/pkg/api"
)

const (
	accept          = "Accept"
	authorization   = "Authorization"
	cacheTTL        = "X-GH-CACHE-TTL"
	graphqlFeatures = "GraphQL-Features"
	features        = "merge_queue"
	userAgent       = "User-Agent"
)

var linkRE = regexp.MustCompile(`<([^>]+)>;\s*rel="([^"]+)"`)

func NewClientFromHTTP(httpClient *http.Client) *Client {
	client := &Client{http: httpClient}
	return client
}

type Client struct {
	http *http.Client
}

func (c *Client) HTTP() *http.Client {
	return c.http
}

type GraphQLError struct {
	ghAPI.GQLError
}

type HTTPError struct {
	ghAPI.HTTPError
	scopesSuggestion string
}

func (err HTTPError) ScopesSuggestion() string {
	return err.scopesSuggestion
}

// GraphQL performs a GraphQL request and parses the response. If there are errors in the response,
// GraphQLError will be returned, but the data will also be parsed into the receiver.
func (c Client) GraphQL(hostname string, query string, variables map[string]interface{}, data interface{}) error {
	opts := clientOptions(hostname, c.http.Transport)
	opts.Headers[graphqlFeatures] = features
	gqlClient, err := gh.GQLClient(&opts)
	if err != nil {
		return err
	}
	return handleResponse(gqlClient.Do(query, variables, data))
}

// GraphQL performs a GraphQL mutation and parses the response. If there are errors in the response,
// GraphQLError will be returned, but the data will also be parsed into the receiver.
func (c Client) Mutate(hostname, name string, mutation interface{}, variables map[string]interface{}) error {
	opts := clientOptions(hostname, c.http.Transport)
	opts.Headers[graphqlFeatures] = features
	gqlClient, err := gh.GQLClient(&opts)
	if err != nil {
		return err
	}
	return handleResponse(gqlClient.Mutate(name, mutation, variables))
}

// GraphQL performs a GraphQL query and parses the response. If there are errors in the response,
// GraphQLError will be returned, but the data will also be parsed into the receiver.
func (c Client) Query(hostname, name string, query interface{}, variables map[string]interface{}) error {
	opts := clientOptions(hostname, c.http.Transport)
	opts.Headers[graphqlFeatures] = features
	gqlClient, err := gh.GQLClient(&opts)
	if err != nil {
		return err
	}
	return handleResponse(gqlClient.Query(name, query, variables))
}

// REST performs a REST request and parses the response.
func (c Client) REST(hostname string, method string, p string, body io.Reader, data interface{}) error {
	opts := clientOptions(hostname, c.http.Transport)
	restClient, err := gh.RESTClient(&opts)
	if err != nil {
		return err
	}
	return handleResponse(restClient.Do(method, p, body, data))
}

func (c Client) RESTWithNext(hostname string, method string, p string, body io.Reader, data interface{}) (string, error) {
	opts := clientOptions(hostname, c.http.Transport)
	restClient, err := gh.RESTClient(&opts)
	if err != nil {
		return "", err
	}

	resp, err := restClient.Request(method, p, body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	if !success {
		return "", HandleHTTPError(resp)
	}

	if resp.StatusCode == http.StatusNoContent {
		return "", nil
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	err = json.Unmarshal(b, &data)
	if err != nil {
		return "", err
	}

	var next string
	for _, m := range linkRE.FindAllStringSubmatch(resp.Header.Get("Link"), -1) {
		if len(m) > 2 && m[2] == "next" {
			next = m[1]
		}
	}

	return next, nil
}

// HandleHTTPError parses a http.Response into a HTTPError.
func HandleHTTPError(resp *http.Response) error {
	return handleResponse(ghAPI.HandleHTTPError(resp))
}

// handleResponse takes a ghAPI.HTTPError or ghAPI.GQLError and converts it into an
// HTTPError or GraphQLError respectively.
func handleResponse(err error) error {
	if err == nil {
		return nil
	}

	var restErr ghAPI.HTTPError
	if errors.As(err, &restErr) {
		return HTTPError{
			HTTPError: restErr,
			scopesSuggestion: generateScopesSuggestion(restErr.StatusCode,
				restErr.Headers.Get("X-Accepted-Oauth-Scopes"),
				restErr.Headers.Get("X-Oauth-Scopes"),
				restErr.RequestURL.Hostname()),
		}
	}

	var gqlErr ghAPI.GQLError
	if errors.As(err, &gqlErr) {
		return GraphQLError{
			GQLError: gqlErr,
		}
	}

	return err
}

// ScopesSuggestion is an error messaging utility that prints the suggestion to request additional OAuth
// scopes in case a server response indicates that there are missing scopes.
func ScopesSuggestion(resp *http.Response) string {
	return generateScopesSuggestion(resp.StatusCode,
		resp.Header.Get("X-Accepted-Oauth-Scopes"),
		resp.Header.Get("X-Oauth-Scopes"),
		resp.Request.URL.Hostname())
}

// EndpointNeedsScopes adds additional OAuth scopes to an HTTP response as if they were returned from the
// server endpoint. This improves HTTP 4xx error messaging for endpoints that don't explicitly list the
// OAuth scopes they need.
func EndpointNeedsScopes(resp *http.Response, s string) *http.Response {
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		oldScopes := resp.Header.Get("X-Accepted-Oauth-Scopes")
		resp.Header.Set("X-Accepted-Oauth-Scopes", fmt.Sprintf("%s, %s", oldScopes, s))
	}
	return resp
}

func generateScopesSuggestion(statusCode int, endpointNeedsScopes, tokenHasScopes, hostname string) string {
	if statusCode < 400 || statusCode > 499 || statusCode == 422 {
		return ""
	}

	if tokenHasScopes == "" {
		return ""
	}

	gotScopes := map[string]struct{}{}
	for _, s := range strings.Split(tokenHasScopes, ",") {
		s = strings.TrimSpace(s)
		gotScopes[s] = struct{}{}

		// Certain scopes may be grouped under a single "top-level" scope. The following branch
		// statements include these grouped/implied scopes when the top-level scope is encountered.
		// See https://docs.github.com/en/developers/apps/building-oauth-apps/scopes-for-oauth-apps.
		if s == "repo" {
			gotScopes["repo:status"] = struct{}{}
			gotScopes["repo_deployment"] = struct{}{}
			gotScopes["public_repo"] = struct{}{}
			gotScopes["repo:invite"] = struct{}{}
			gotScopes["security_events"] = struct{}{}
		} else if s == "user" {
			gotScopes["read:user"] = struct{}{}
			gotScopes["user:email"] = struct{}{}
			gotScopes["user:follow"] = struct{}{}
		} else if s == "codespace" {
			gotScopes["codespace:secrets"] = struct{}{}
		} else if strings.HasPrefix(s, "admin:") {
			gotScopes["read:"+strings.TrimPrefix(s, "admin:")] = struct{}{}
			gotScopes["write:"+strings.TrimPrefix(s, "admin:")] = struct{}{}
		} else if strings.HasPrefix(s, "write:") {
			gotScopes["read:"+strings.TrimPrefix(s, "write:")] = struct{}{}
		}
	}

	for _, s := range strings.Split(endpointNeedsScopes, ",") {
		s = strings.TrimSpace(s)
		if _, gotScope := gotScopes[s]; s == "" || gotScope {
			continue
		}
		return fmt.Sprintf(
			"This API operation needs the %[1]q scope. To request it, run:  gh auth refresh -h %[2]s -s %[1]s",
			s,
			ghinstance.NormalizeHostname(hostname),
		)
	}

	return ""
}

func clientOptions(hostname string, transport http.RoundTripper) ghAPI.ClientOptions {
	// AuthToken, and Headers are being handled by transport,
	// so let go-gh know that it does not need to resolve them.
	opts := ghAPI.ClientOptions{
		AuthToken: "none",
		Headers: map[string]string{
			authorization: "",
		},
		Host:               hostname,
		SkipDefaultHeaders: true,
		Transport:          transport,
		LogIgnoreEnv:       true,
	}
	return opts
}
