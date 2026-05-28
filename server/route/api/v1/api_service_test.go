package v1

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	v1pb "github.com/devlopali-dev/slash/proto/gen/api/v1"
	storepb "github.com/devlopali-dev/slash/proto/gen/store"
	"github.com/devlopali-dev/slash/server/profile"
	teststore "github.com/devlopali-dev/slash/store/test"
)

// fakeStream implements grpc.ServerTransportStream for tests.
// It absorbs SetHeader/SendHeader/SetTrailer calls that gRPC handlers emit
// (e.g. Set-Cookie on sign-in).
type fakeStream struct{ headers metadata.MD }

func (s *fakeStream) Method() string { return "" }
func (s *fakeStream) SetHeader(md metadata.MD) error {
	s.headers = metadata.Join(s.headers, md)
	return nil
}
func (s *fakeStream) SendHeader(md metadata.MD) error { return nil }
func (s *fakeStream) SetTrailer(md metadata.MD) error { return nil }

// grpcCtx returns a context carrying a fake gRPC server transport stream.
// Required for any handler that calls grpc.SetHeader (SignIn, SignUp, SignOut).
func grpcCtx(t *testing.T) context.Context {
	t.Helper()
	return grpc.NewContextWithServerTransportStream(context.Background(), &fakeStream{})
}

// authedCtx wraps grpcCtx and injects a user ID so getCurrentUser returns the
// given user, simulating an already-authenticated request.
func authedCtx(t *testing.T, userID int32) context.Context {
	t.Helper()
	return context.WithValue(grpcCtx(t), userIDContextKey, userID)
}

// newTestService creates a fresh APIV1Service backed by an in-memory SQLite store.
func newTestService(ctx context.Context, t *testing.T) *APIV1Service {
	t.Helper()
	ts := teststore.NewTestingStore(ctx, t)
	return &APIV1Service{
		Secret:  "test-secret-32-bytes-padded!!!",
		Profile: &profile.Profile{Mode: "dev"},
		Store:   ts,
	}
}

// signUpUser is a test helper that registers a user and returns the created user proto.
func signUpUser(ctx context.Context, t *testing.T, svc *APIV1Service, email, password string) *v1pb.User {
	t.Helper()
	user, err := svc.SignUp(ctx, &v1pb.SignUpRequest{
		Email:    email,
		Nickname: email,
		Password: password,
	})
	require.NoError(t, err)
	return user
}

// ── Auth ──────────────────────────────────────────────────────────────────────

func TestSignUp_FirstUserBecomesAdmin(t *testing.T) {
	ctx := grpcCtx(t)
	svc := newTestService(ctx, t)

	user, err := svc.SignUp(ctx, &v1pb.SignUpRequest{
		Email:    "admin@example.com",
		Nickname: "admin",
		Password: "password123",
	})

	require.NoError(t, err)
	require.Equal(t, v1pb.Role_ADMIN, user.Role)
}

func TestSignUp_SubsequentUsersAreRegular(t *testing.T) {
	ctx := grpcCtx(t)
	svc := newTestService(ctx, t)

	signUpUser(ctx, t, svc, "admin@example.com", "password123")

	second, err := svc.SignUp(ctx, &v1pb.SignUpRequest{
		Email:    "user@example.com",
		Nickname: "user",
		Password: "password123",
	})

	require.NoError(t, err)
	require.Equal(t, v1pb.Role_USER, second.Role)
}

func TestSignUp_PasswordTooShort(t *testing.T) {
	ctx := grpcCtx(t)
	svc := newTestService(ctx, t)

	_, err := svc.SignUp(ctx, &v1pb.SignUpRequest{
		Email:    "a@example.com",
		Password: "short",
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "at least 8")
}

func TestSignUp_PasswordTooLong(t *testing.T) {
	ctx := grpcCtx(t)
	svc := newTestService(ctx, t)

	_, err := svc.SignUp(ctx, &v1pb.SignUpRequest{
		Email:    "a@example.com",
		Password: strings.Repeat("x", 73),
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "at most 72")
}

func TestSignUp_DuplicateEmailRejected(t *testing.T) {
	ctx := grpcCtx(t)
	svc := newTestService(ctx, t)

	signUpUser(ctx, t, svc, "dup@example.com", "password123")

	_, err := svc.SignUp(ctx, &v1pb.SignUpRequest{
		Email:    "dup@example.com",
		Nickname: "dup2",
		Password: "password123",
	})

	require.Error(t, err)
}

func TestSignIn_CorrectCredentials(t *testing.T) {
	ctx := grpcCtx(t)
	svc := newTestService(ctx, t)

	signUpUser(ctx, t, svc, "user@example.com", "password123")

	user, err := svc.SignIn(ctx, &v1pb.SignInRequest{
		Email:    "user@example.com",
		Password: "password123",
	})

	require.NoError(t, err)
	require.Equal(t, "user@example.com", user.Email)
}

func TestSignIn_WrongPassword(t *testing.T) {
	ctx := grpcCtx(t)
	svc := newTestService(ctx, t)

	signUpUser(ctx, t, svc, "user@example.com", "password123")

	_, err := svc.SignIn(ctx, &v1pb.SignInRequest{
		Email:    "user@example.com",
		Password: "wrongpassword",
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "unmatched email and password")
}

func TestSignIn_UnknownEmail(t *testing.T) {
	ctx := grpcCtx(t)
	svc := newTestService(ctx, t)

	_, err := svc.SignIn(ctx, &v1pb.SignInRequest{
		Email:    "nobody@example.com",
		Password: "password123",
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "unmatched email and password")
}

// ── Shortcut ──────────────────────────────────────────────────────────────────

func TestListShortcuts_UnauthenticatedSeesOnlyPublic(t *testing.T) {
	ctx := grpcCtx(t)
	svc := newTestService(ctx, t)

	admin := signUpUser(ctx, t, svc, "admin@example.com", "password123")
	adminCtx := authedCtx(t, admin.Id)

	// Bypass DNS validation: insert shortcuts directly via the store.
	_, err := svc.Store.CreateShortcut(ctx, &storepb.Shortcut{
		CreatorId:  admin.Id,
		Name:       "pub",
		Link:       "https://example.com",
		Visibility: storepb.Visibility_PUBLIC,
		OgMetadata: &storepb.OpenGraphMetadata{},
	})
	require.NoError(t, err)

	_, err = svc.Store.CreateShortcut(ctx, &storepb.Shortcut{
		CreatorId:  admin.Id,
		Name:       "ws",
		Link:       "https://example.com",
		Visibility: storepb.Visibility_WORKSPACE,
		OgMetadata: &storepb.OpenGraphMetadata{},
	})
	require.NoError(t, err)

	// Unauthenticated request — must see only PUBLIC.
	resp, err := svc.ListShortcuts(grpcCtx(t), &v1pb.ListShortcutsRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Shortcuts, 1)
	require.Equal(t, "pub", resp.Shortcuts[0].Name)

	// Authenticated member sees both.
	resp, err = svc.ListShortcuts(adminCtx, &v1pb.ListShortcutsRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Shortcuts, 2)
}

func TestListShortcuts_AuthenticatedSeesWorkspaceAndPublic(t *testing.T) {
	ctx := grpcCtx(t)
	svc := newTestService(ctx, t)

	admin := signUpUser(ctx, t, svc, "admin@example.com", "password123")
	adminCtx := authedCtx(t, admin.Id)

	for _, sc := range []struct {
		name string
		vis  storepb.Visibility
	}{
		{"pub", storepb.Visibility_PUBLIC},
		{"ws", storepb.Visibility_WORKSPACE},
	} {
		_, err := svc.Store.CreateShortcut(ctx, &storepb.Shortcut{
			CreatorId:  admin.Id,
			Name:       sc.name,
			Link:       "https://example.com",
			Visibility: sc.vis,
			OgMetadata: &storepb.OpenGraphMetadata{},
		})
		require.NoError(t, err)
	}

	resp, err := svc.ListShortcuts(adminCtx, &v1pb.ListShortcutsRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Shortcuts, 2)
}

func TestListShortcuts_NoIDORLeakViaVisibilityUnspecified(t *testing.T) {
	ctx := grpcCtx(t)
	svc := newTestService(ctx, t)

	owner := signUpUser(ctx, t, svc, "owner@example.com", "password123")
	other := signUpUser(grpcCtx(t), t, svc, "other@example.com", "password123")

	// Insert a legacy PRIVATE row directly via SQL, bypassing the Go store layer
	// and the CHECK constraint (which now only allows WORKSPACE/PUBLIC on new DBs).
	// This simulates data migrated from an older schema version.
	db := svc.Store.GetDB()
	_, err := db.ExecContext(ctx,
		`INSERT INTO shortcut (creator_id, name, link, title, description, visibility, tag, og_metadata)
		 VALUES (?, ?, ?, '', '', 'PRIVATE', '', '{}')`,
		owner.Id, "private-sc", "https://secret.example.com",
	)
	// If the schema rejects PRIVATE (fresh install), the test skips — the constraint
	// itself is the protection. If it succeeds (migrated DB), verify ListShortcuts filters it.
	if err != nil {
		t.Skipf("schema rejects PRIVATE visibility (expected on fresh install): %v", err)
	}

	resp, err := svc.ListShortcuts(authedCtx(t, other.Id), &v1pb.ListShortcutsRequest{})
	require.NoError(t, err)
	for _, sc := range resp.Shortcuts {
		require.NotEqual(t, "private-sc", sc.Name, "IDOR: non-owner can see PRIVATE shortcut")
	}
}

func TestCreateShortcut_RejectsPrivateIPLink(t *testing.T) {
	ctx := grpcCtx(t)
	svc := newTestService(ctx, t)

	admin := signUpUser(ctx, t, svc, "admin@example.com", "password123")
	adminCtx := authedCtx(t, admin.Id)

	_, err := svc.CreateShortcut(adminCtx, &v1pb.CreateShortcutRequest{
		Shortcut: &v1pb.Shortcut{
			Name: "internal",
			Link: "http://192.168.1.1/admin",
		},
	})

	require.Error(t, err)
}

func TestDeleteShortcut_OwnerCanDelete(t *testing.T) {
	ctx := grpcCtx(t)
	svc := newTestService(ctx, t)

	admin := signUpUser(ctx, t, svc, "admin@example.com", "password123")

	sc, err := svc.Store.CreateShortcut(ctx, &storepb.Shortcut{
		CreatorId:  admin.Id,
		Name:       "todelete",
		Link:       "https://example.com",
		Visibility: storepb.Visibility_WORKSPACE,
		OgMetadata: &storepb.OpenGraphMetadata{},
	})
	require.NoError(t, err)

	_, err = svc.DeleteShortcut(authedCtx(t, admin.Id), &v1pb.DeleteShortcutRequest{Id: sc.Id})
	require.NoError(t, err)
}

func TestDeleteShortcut_NonOwnerCannotDelete(t *testing.T) {
	ctx := grpcCtx(t)
	svc := newTestService(ctx, t)

	admin := signUpUser(ctx, t, svc, "admin@example.com", "password123")
	other := signUpUser(grpcCtx(t), t, svc, "other@example.com", "password123")

	sc, err := svc.Store.CreateShortcut(ctx, &storepb.Shortcut{
		CreatorId:  admin.Id,
		Name:       "owned",
		Link:       "https://example.com",
		Visibility: storepb.Visibility_WORKSPACE,
		OgMetadata: &storepb.OpenGraphMetadata{},
	})
	require.NoError(t, err)

	_, err = svc.DeleteShortcut(authedCtx(t, other.Id), &v1pb.DeleteShortcutRequest{Id: sc.Id})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Permission denied")
}

// ── User ──────────────────────────────────────────────────────────────────────

func TestGetUser_EmailScrubbedForNonOwner(t *testing.T) {
	ctx := grpcCtx(t)
	svc := newTestService(ctx, t)

	admin := signUpUser(ctx, t, svc, "admin@example.com", "password123")
	other := signUpUser(grpcCtx(t), t, svc, "other@example.com", "password123")

	// 'other' fetches 'admin' — email must be blank.
	user, err := svc.GetUser(authedCtx(t, other.Id), &v1pb.GetUserRequest{Id: admin.Id})
	require.NoError(t, err)
	require.Empty(t, user.Email)
}

func TestGetUser_OwnerSeesOwnEmail(t *testing.T) {
	ctx := grpcCtx(t)
	svc := newTestService(ctx, t)

	admin := signUpUser(ctx, t, svc, "admin@example.com", "password123")

	user, err := svc.GetUser(authedCtx(t, admin.Id), &v1pb.GetUserRequest{Id: admin.Id})
	require.NoError(t, err)
	require.Equal(t, "admin@example.com", user.Email)
}

func TestCreateUser_PasswordBounds(t *testing.T) {
	ctx := grpcCtx(t)
	svc := newTestService(ctx, t)

	admin := signUpUser(ctx, t, svc, "admin@example.com", "password123")
	adminCtx := authedCtx(t, admin.Id)

	_, err := svc.CreateUser(adminCtx, &v1pb.CreateUserRequest{
		User: &v1pb.User{Email: "a@b.com", Password: "short"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "at least 8")

	_, err = svc.CreateUser(adminCtx, &v1pb.CreateUserRequest{
		User: &v1pb.User{Email: "a@b.com", Password: strings.Repeat("x", 73)},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "at most 72")
}
