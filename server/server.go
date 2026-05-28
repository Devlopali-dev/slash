package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/pkg/errors"

	storepb "github.com/devlopali-dev/slash/proto/gen/store"
	"github.com/devlopali-dev/slash/server/profile"
	apiv1 "github.com/devlopali-dev/slash/server/route/api/v1"
	"github.com/devlopali-dev/slash/server/route/frontend"
	"github.com/devlopali-dev/slash/server/runner/version"
	"github.com/devlopali-dev/slash/store"
)

type Server struct {
	e *echo.Echo

	Profile *profile.Profile
	Store   *store.Store
	Secret  string

	// API services.
	apiV1Service *apiv1.APIV1Service
}

func NewServer(ctx context.Context, profile *profile.Profile, store *store.Store) (*Server, error) {
	e := echo.New()
	e.Debug = profile.Mode != "prod"
	e.HideBanner = true
	e.HidePort = true

	// Security headers middleware.
	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Response().Header().Set("X-Content-Type-Options", "nosniff")
			c.Response().Header().Set("X-Frame-Options", "DENY")
			c.Response().Header().Set("X-XSS-Protection", "1; mode=block")
			c.Response().Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			c.Response().Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; font-src 'self' data:; connect-src 'self'; frame-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
			c.Response().Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			c.Response().Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			return next(c)
		}
	})

	s := &Server{
		e:       e,
		Profile: profile,
		Store:   store,
	}

	// Serve frontend.
	frontendService := frontend.NewFrontendService(profile, store)
	frontendService.Serve(ctx, e)

	secret, err := s.getSecretSession(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get secret session")
	}
	s.Secret = secret

	// Register healthz endpoint.
	e.GET("/healthz", func(c echo.Context) error {
		return c.String(http.StatusOK, "Service ready.")
	})

	s.apiV1Service = apiv1.NewAPIV1Service(secret, profile, store, s.Profile.Port+1)
	// Register gRPC gateway as api v1.
	if err := s.apiV1Service.RegisterGateway(ctx, e); err != nil {
		return nil, errors.Wrap(err, "failed to register gRPC gateway")
	}

	return s, nil
}

func (s *Server) Start(ctx context.Context) error {
	s.StartBackgroundRunners(ctx)
	// Start gRPC server.
	listen, err := net.Listen("tcp", fmt.Sprintf(":%d", s.Profile.Port+1))
	if err != nil {
		return err
	}
	go func() {
		if err := s.apiV1Service.GetGRPCServer().Serve(listen); err != nil {
			slog.Log(ctx, slog.LevelError, "failed to start grpc server")
		}
	}()

	return s.e.Start(fmt.Sprintf(":%d", s.Profile.Port))
}

func (s *Server) Shutdown(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Shutdown echo server.
	if err := s.e.Shutdown(ctx); err != nil {
		fmt.Printf("failed to shutdown server, error: %v\n", err)
	}

	// Close database connection.
	if err := s.Store.Close(); err != nil {
		fmt.Printf("failed to close database, error: %v\n", err)
	}

	fmt.Println("server stopped properly")
}

func (s *Server) GetEcho() *echo.Echo {
	return s.e
}

func (s *Server) StartBackgroundRunners(ctx context.Context) {
	versionRunner := version.NewRunner(s.Store, s.Profile)
	versionRunner.RunOnce(ctx)
	go versionRunner.Run(ctx)
}

func (s *Server) getSecretSession(ctx context.Context) (string, error) {
	workspaceGeneralSetting, err := s.Store.GetWorkspaceGeneralSetting(ctx)
	if err != nil {
		return "", err
	}
	secretSession := workspaceGeneralSetting.SecretSession
	if secretSession == "" {
		secretSession = uuid.New().String()
		_, err := s.Store.UpsertWorkspaceSetting(ctx, &storepb.WorkspaceSetting{
			Key: storepb.WorkspaceSettingKey_WORKSPACE_SETTING_GENERAL,
			Value: &storepb.WorkspaceSetting_General{
				General: &storepb.WorkspaceSetting_GeneralSetting{
					SecretSession: secretSession,
				},
			},
		})
		if err != nil {
			return "", err
		}
	}
	return secretSession, nil
}
