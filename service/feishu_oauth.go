package service

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkauthen "github.com/larksuite/oapi-sdk-go/v3/service/authen/v1"
	"github.com/lejianwen/rustdesk-api/v2/model"
	log "github.com/sirupsen/logrus"
)

const feishuAuthorizeURL = "https://accounts.feishu.cn/open-apis/authen/v1/authorize"

const feishuHTTPTimeout = 60 * time.Second

type feishuClientFactory func(appID, appSecret string, httpClient larkcore.HttpClient, openBaseURL string) *lark.Client

var newFeishuClient feishuClientFactory = func(appID, appSecret string, httpClient larkcore.HttpClient, openBaseURL string) *lark.Client {
	options := []lark.ClientOptionFunc{lark.WithHttpClient(httpClient)}
	if openBaseURL != "" {
		options = append(options, lark.WithOpenBaseUrl(openBaseURL), lark.WithOAuthBaseUrl(openBaseURL))
	}
	return lark.NewClient(appID, appSecret, options...)
}

func buildFeishuAuthorizationURL(info *model.Oauth, state, redirectURL string) (string, error) {
	if strings.TrimSpace(info.ClientId) == "" {
		return "", errors.New("ConfigNotFound")
	}

	u, err := url.Parse(feishuAuthorizeURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("app_id", info.ClientId)
	q.Set("redirect_uri", redirectURL)
	q.Set("state", state)
	if scopes := strings.Join(constructFeishuScopes(info.Scopes), " "); scopes != "" {
		q.Set("scope", scopes)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func constructFeishuScopes(scopes string) []string {
	if strings.TrimSpace(scopes) == "" {
		return []string{}
	}

	values := strings.Split(scopes, ",")
	result := make([]string, 0, len(values))
	for _, scope := range values {
		if scope = strings.TrimSpace(scope); scope != "" {
			result = append(result, scope)
		}
	}
	return result
}

func (os *OauthService) feishuCallback(info *model.Oauth, code string) (error, *model.OauthUser) {
	client := newFeishuClient(info.ClientId, info.ClientSecret, getFeishuHTTPClient(), "")
	ctx := context.Background()
	tokenResp, err := client.Authen.V1.AccessToken.Create(ctx,
		larkauthen.NewCreateAccessTokenReqBuilder().
			Body(larkauthen.NewCreateAccessTokenReqBodyBuilder().
				GrantType("authorization_code").
				Code(code).
				Build()).
			Build(),
	)
	if err != nil {
		logFeishuOAuthFailure("GetOauthTokenError", 0, "")
		return errors.New("GetOauthTokenError"), nil
	}
	if tokenResp == nil || !tokenResp.Success() || tokenResp.Data == nil || tokenResp.Data.AccessToken == nil || strings.TrimSpace(*tokenResp.Data.AccessToken) == "" {
		if tokenResp == nil {
			logFeishuOAuthFailure("GetOauthTokenError", 0, "")
		} else {
			logFeishuOAuthFailure("GetOauthTokenError", tokenResp.Code, tokenResp.RequestId())
		}
		return errors.New("GetOauthTokenError"), nil
	}

	userResp, err := client.Authen.V1.UserInfo.Get(ctx, larkcore.WithUserAccessToken(*tokenResp.Data.AccessToken))
	if err != nil {
		logFeishuOAuthFailure("DecodeOauthUserInfoError", 0, "")
		return errors.New("DecodeOauthUserInfoError"), nil
	}
	if userResp == nil || !userResp.Success() || userResp.Data == nil {
		if userResp == nil {
			logFeishuOAuthFailure("GetOauthUserInfoError", 0, "")
		} else {
			logFeishuOAuthFailure("GetOauthUserInfoError", userResp.Code, userResp.RequestId())
		}
		return errors.New("GetOauthUserInfoError"), nil
	}

	openID := feishuString(userResp.Data.OpenId)
	if openID == "" {
		logFeishuOAuthFailure("DecodeOauthUserInfoError", userResp.Code, userResp.RequestId())
		return errors.New("DecodeOauthUserInfoError"), nil
	}
	email := feishuString(userResp.Data.Email)
	if email == "" {
		email = feishuString(userResp.Data.EnterpriseEmail)
	}

	return nil, (&model.FeishuUser{
		OpenID:    openID,
		Name:      feishuString(userResp.Data.Name),
		Email:     email,
		AvatarURL: feishuString(userResp.Data.AvatarUrl),
	}).ToOauthUser()
}

func getFeishuHTTPClient() *http.Client {
	client := getHTTPClientWithProxy()
	if client.Timeout > 0 {
		return client
	}
	boundedClient := *client
	boundedClient.Timeout = feishuHTTPTimeout
	return &boundedClient
}

func feishuString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func logFeishuOAuthFailure(errorClass string, code int, requestID string) {
	if Logger == nil {
		return
	}
	Logger.WithFields(log.Fields{
		"error_class": errorClass,
		"feishu_code": code,
		"request_id":  requestID,
	}).Warn("Feishu OAuth request failed")
}
