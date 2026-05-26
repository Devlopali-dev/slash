// Package oauth2 is the plugin for OAuth2 Identity Provider.
package oauth2

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/oauth2"

	"github.com/Devlopali-dev/slash/internal/util"
	"github.com/Devlopali-dev/slash/plugin/idp"
	storepb "github.com/Devlopali-dev/slash/proto/gen/store"
)

// IdentityProvider represents an OAuth2 Identity Provider.
type IdentityProvider struct {
	config *storepb.IdentityProviderConfig_OAuth2Config
}

// NewIdentityProvider initializes a new OAuth2 Identity Provider with the given configuration.
func NewIdentityProvider(config *storepb.IdentityProviderConfig_OAuth2Config) (*IdentityProvider, error) {
	for v, field := range map[string]string{
		config.ClientId:                "clientId",
		config.ClientSecret:            "clientSecret",
		config.TokenUrl:                "tokenUrl",
		config.UserInfoUrl:             "userInfoUrl",
		config.FieldMapping.Identifier: "fieldMapping.identifier",
	} {
		if v == "" {
			return nil, errors.Errorf(`the field "%s" is empty but required`, field)
		}
	}

	for rawURL, field := range map[string]string{
		config.TokenUrl:    "tokenUrl",
		config.UserInfoUrl: "userInfoUrl",
	} {
		if err := util.ValidateHTTPURL(rawURL); err != nil {
			return nil, errors.Errorf(`the field "%s" is invalid: %v`, field, err)
		}
	}
	if config.AuthUrl != "" {
		if err := util.ValidateHTTPURL(config.AuthUrl); err != nil {
			return nil, errors.Errorf(`the field "authUrl" is invalid: %v`, err)
		}
	}

	return &IdentityProvider{
		config: config,
	}, nil
}

// ExchangeToken returns the exchanged OAuth2 token using the given authorization code.
func (p *IdentityProvider) ExchangeToken(ctx context.Context, redirectURL, code string) (string, error) {
	conf := &oauth2.Config{
		ClientID:     p.config.ClientId,
		ClientSecret: p.config.ClientSecret,
		RedirectURL:  redirectURL,
		Scopes:       p.config.Scopes,
		Endpoint: oauth2.Endpoint{
			AuthURL:   p.config.AuthUrl,
			TokenURL:  p.config.TokenUrl,
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}

	token, err := conf.Exchange(ctx, code)
	if err != nil {
		return "", errors.Wrap(err, "failed to exchange access token")
	}

	accessToken, ok := token.Extra("access_token").(string)
	if !ok {
		return "", errors.New(`missing "access_token" from authorization response`)
	}

	return accessToken, nil
}

const maxUserInfoBodyBytes = 1 << 20 // 1 MiB

// UserInfo returns the parsed user information using the given OAuth2 token.
func (p *IdentityProvider) UserInfo(token string) (*idp.IdentityProviderUserInfo, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(http.MethodGet, p.config.UserInfoUrl, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to new http request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get user information")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxUserInfoBodyBytes))
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response body")
	}

	var claims map[string]any
	err = json.Unmarshal(body, &claims)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal response body")
	}

	userInfo := &idp.IdentityProviderUserInfo{}
	if v, ok := claims[p.config.FieldMapping.Identifier].(string); ok {
		userInfo.Identifier = v
	}
	if userInfo.Identifier == "" {
		return nil, errors.Errorf("the field %q is not found in claims or has empty value", p.config.FieldMapping.Identifier)
	}

	// Best effort to map optional fields.
	if p.config.FieldMapping.DisplayName != "" {
		if v, ok := claims[p.config.FieldMapping.DisplayName].(string); ok {
			userInfo.DisplayName = v
		}
	}
	if userInfo.DisplayName == "" {
		userInfo.DisplayName = userInfo.Identifier
	}
	return userInfo, nil
}
