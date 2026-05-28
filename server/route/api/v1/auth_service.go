package v1

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/devlopali-dev/slash/internal/util"
	"github.com/devlopali-dev/slash/plugin/idp"
	"github.com/devlopali-dev/slash/plugin/idp/oauth2"
	v1pb "github.com/devlopali-dev/slash/proto/gen/api/v1"
	storepb "github.com/devlopali-dev/slash/proto/gen/store"
	"github.com/devlopali-dev/slash/store"
)

const (
	unmatchedEmailAndPasswordError = "unmatched email and password"
)

func (s *APIV1Service) GetAuthStatus(ctx context.Context, _ *v1pb.GetAuthStatusRequest) (*v1pb.User, error) {
	user, err := getCurrentUser(ctx, s.Store)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get current user: %v", err)
	}
	if user == nil {
		return nil, status.Errorf(codes.Unauthenticated, "user not found")
	}
	return convertUserFromStore(user), nil
}

func (s *APIV1Service) SignIn(ctx context.Context, request *v1pb.SignInRequest) (*v1pb.User, error) {
	if err := checkAuthRateLimit(ctx); err != nil {
		return nil, err
	}
	user, err := s.Store.GetUser(ctx, &store.FindUser{
		Email: &request.Email,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user: %v", err)
	}
	if user == nil {
		return nil, status.Errorf(codes.InvalidArgument, unmatchedEmailAndPasswordError)
	}
	// Compare the stored hashed password, with the hashed version of the password that was received.
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(request.Password)); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, unmatchedEmailAndPasswordError)
	}

	workspaceSecuritySetting, err := s.Store.GetWorkspaceSecuritySetting(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get workspace security setting: %v", err)
	}
	if workspaceSecuritySetting.DisallowPasswordAuth && user.Role == store.RoleUser {
		return nil, status.Errorf(codes.PermissionDenied, "password authentication is not allowed")
	}
	if user.RowStatus == storepb.RowStatus_ARCHIVED {
		return nil, status.Errorf(codes.PermissionDenied, "user has been archived")
	}

	if err := s.doSignIn(ctx, user, time.Now().Add(AccessTokenDuration)); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to sign in: %v", err)
	}
	return convertUserFromStore(user), nil
}

func (s *APIV1Service) SignInWithSSO(ctx context.Context, request *v1pb.SignInWithSSORequest) (*v1pb.User, error) {
	if err := checkAuthRateLimit(ctx); err != nil {
		return nil, err
	}
	identityProviderSetting, err := s.Store.GetWorkspaceSetting(ctx, &store.FindWorkspaceSetting{
		Key: storepb.WorkspaceSettingKey_WORKSPACE_SETTING_IDENTITY_PROVIDER,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get workspace setting, err: %s", err)
	}
	if identityProviderSetting == nil || identityProviderSetting.GetIdentityProvider() == nil {
		return nil, status.Errorf(codes.InvalidArgument, "identity provider not found")
	}
	var identityProvider *storepb.IdentityProvider
	for _, idp := range identityProviderSetting.GetIdentityProvider().IdentityProviders {
		if idp.Id == request.IdpId {
			identityProvider = idp
			break
		}
	}
	if identityProvider == nil {
		return nil, status.Errorf(codes.InvalidArgument, "identity provider not found")
	}

	var userInfo *idp.IdentityProviderUserInfo
	if identityProvider.Type == storepb.IdentityProvider_OAUTH2 {
		oauth2IdentityProvider, err := oauth2.NewIdentityProvider(identityProvider.Config.GetOauth2())
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to create oauth2 identity provider, err: %s", err)
		}
		token, err := oauth2IdentityProvider.ExchangeToken(ctx, request.RedirectUri, request.Code)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to exchange token, err: %s", err)
		}
		userInfo, err = oauth2IdentityProvider.UserInfo(token)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to get user info, err: %s", err)
		}
	}

	email := userInfo.Identifier
	if !util.ValidateEmail(email) {
		return nil, status.Errorf(codes.InvalidArgument, "invalid email address")
	}
	user, err := s.Store.GetUser(ctx, &store.FindUser{
		Email: &email,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user, err: %s", err)
	}
	if user == nil {
		if err := s.checkSeatAvailability(ctx); err != nil {
			return nil, err
		}
		userCreate := &store.User{
			Email:    email,
			Nickname: userInfo.DisplayName,
			// The new signup user should be normal user by default.
			Role: store.RoleUser,
		}
		password, err := util.RandomString(20)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to generate random password, err: %s", err)
		}
		passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to generate password hash, err: %s", err)
		}
		userCreate.PasswordHash = string(passwordHash)
		user, err = s.Store.CreateUser(ctx, userCreate)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to create user, err: %s", err)
		}
	}
	if user.RowStatus == storepb.RowStatus_ARCHIVED {
		return nil, status.Errorf(codes.PermissionDenied, "user has been archived")
	}

	if err := s.doSignIn(ctx, user, time.Now().Add(AccessTokenDuration)); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to sign in, err: %s", err)
	}
	return convertUserFromStore(user), nil
}

// signupMu serializes concurrent signups to make the first-user admin
// promotion atomic (check-then-create must not interleave with another signup).
var signupMu sync.Mutex

func (s *APIV1Service) SignUp(ctx context.Context, request *v1pb.SignUpRequest) (*v1pb.User, error) {
	if err := checkAuthRateLimit(ctx); err != nil {
		return nil, err
	}
	workspaceSecuritySetting, err := s.Store.GetWorkspaceSecuritySetting(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get workspace security setting: %v", err)
	}
	if workspaceSecuritySetting.DisallowUserRegistration {
		return nil, status.Errorf(codes.PermissionDenied, "sign up is not allowed")
	}

	if len(request.Password) < 8 {
		return nil, status.Errorf(codes.InvalidArgument, "password must be at least 8 characters")
	}
	if len(request.Password) > 72 {
		return nil, status.Errorf(codes.InvalidArgument, "password must be at most 72 characters")
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(request.Password), 12)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to generate password hash: %v", err)
	}

	// Serialize signups so the count-then-create is atomic: only the first user
	// ever created gets promoted to admin, with no window for a concurrent signup
	// to race past the check.
	signupMu.Lock()
	user, err := func() (*store.User, error) {
		defer signupMu.Unlock()

		if err := s.checkSeatAvailability(ctx); err != nil {
			return nil, err
		}

		existing, err := s.Store.ListUsers(ctx, &store.FindUser{})
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to list users: %v", err)
		}
		role := store.RoleUser
		if len(existing) == 0 {
			role = store.RoleAdmin
		}

		return s.Store.CreateUser(ctx, &store.User{
			Email:        request.Email,
			Nickname:     request.Nickname,
			PasswordHash: string(passwordHash),
			Role:         role,
		})
	}()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create user: %v", err)
	}

	if err := s.doSignIn(ctx, user, time.Now().Add(AccessTokenDuration)); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to sign in: %v", err)
	}
	return convertUserFromStore(user), nil
}

func (s *APIV1Service) doSignIn(ctx context.Context, user *store.User, expireTime time.Time) error {
	accessToken, err := GenerateAccessToken(user.Email, user.ID, expireTime, []byte(s.Secret))
	if err != nil {
		return status.Errorf(codes.Internal, "failed to generate access token: %v", err)
	}
	if err := s.UpsertAccessTokenToStore(ctx, user, accessToken, "user login"); err != nil {
		return status.Errorf(codes.Internal, "failed to upsert access token to store: %v", err)
	}

	cookie := fmt.Sprintf("%s=%s; Path=/; Expires=%s; HttpOnly; SameSite=Strict; Secure", AccessTokenCookieName, accessToken, time.Now().Add(AccessTokenDuration).Format(time.RFC1123))
	if err := grpc.SetHeader(ctx, metadata.New(map[string]string{
		"Set-Cookie": cookie,
	})); err != nil {
		return status.Errorf(codes.Internal, "failed to set grpc header, error: %v", err)
	}

	return nil
}

func (s *APIV1Service) SignOut(ctx context.Context, _ *v1pb.SignOutRequest) (*emptypb.Empty, error) {
	// Revoke the token from the store so it cannot be reused after logout.
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		accessToken, err := getTokenFromMetadata(md)
		if err == nil && accessToken != "" {
			user, err := getCurrentUser(ctx, s.Store)
			if err == nil && user != nil {
				if err := s.deleteAccessTokenFromStore(ctx, user, accessToken); err != nil {
					return nil, status.Errorf(codes.Internal, "failed to revoke access token: %v", err)
				}
			}
		}
	}

	// Expire the cookie on the client side.
	if err := grpc.SetHeader(ctx, metadata.New(map[string]string{
		"Set-Cookie": fmt.Sprintf("%s=; Path=/; Expires=Thu, 01 Jan 1970 00:00:00 GMT; HttpOnly; SameSite=Strict", AccessTokenCookieName),
	})); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to set grpc header, error: %v", err)
	}
	return &emptypb.Empty{}, nil
}

func (s *APIV1Service) deleteAccessTokenFromStore(ctx context.Context, user *store.User, accessToken string) error {
	tokens, err := s.Store.GetUserAccessTokens(ctx, user.ID)
	if err != nil {
		return err
	}
	updated := make([]*storepb.UserSetting_AccessTokensSetting_AccessToken, 0, len(tokens))
	for _, t := range tokens {
		if t.AccessToken != accessToken {
			updated = append(updated, t)
		}
	}
	_, err = s.Store.UpsertUserSetting(ctx, &storepb.UserSetting{
		UserId: user.ID,
		Key:    storepb.UserSettingKey_USER_SETTING_ACCESS_TOKENS,
		Value: &storepb.UserSetting_AccessTokens{
			AccessTokens: &storepb.UserSetting_AccessTokensSetting{
				AccessTokens: updated,
			},
		},
	})
	return err
}

func (_ *APIV1Service) checkSeatAvailability(_ context.Context) error {
	return nil
}
