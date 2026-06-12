package httpapi

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"

	"hermes-opencode-team/orchestrator/internal/auth"
	"hermes-opencode-team/orchestrator/internal/codeengine"
	"hermes-opencode-team/orchestrator/internal/config"
	"hermes-opencode-team/orchestrator/internal/memory"
	"hermes-opencode-team/orchestrator/internal/workflow"
)

type MemoryStore interface {
	Ping(ctx context.Context) error
	Recall(ctx context.Context, sessionID string, limit int) ([]memory.Event, error)
}

type WorkflowEngine interface {
	RunWorkflow(ctx context.Context, task, sessionID string, useCodeEngine bool) (workflow.RunResult, error)
	ApproveApproval(ctx context.Context, approvalID, engine string) (codeengine.Result, error)
}

type Server struct {
	echo    *echo.Echo
	cfg     config.Config
	store   MemoryStore
	engine  WorkflowEngine
	ctx     context.Context
	cancel  context.CancelFunc
	started bool
}

type TaskRequest struct {
	Task          string `json:"task"`
	SessionID     string `json:"session_id"`
	UseCodeEngine *bool  `json:"use_code_engine"`
}

type ApproveRequest struct {
	ApprovalID string `json:"approval_id"`
	Engine     string `json:"engine"`
}

func NewServer(cfg config.Config, store MemoryStore, engine WorkflowEngine) *Server {
	e := echo.New()
	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())
	e.Use(middleware.RequestLogger())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodOptions},
		AllowHeaders: []string{echo.HeaderAuthorization, echo.HeaderContentType},
	}))

	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{echo: e, cfg: cfg, store: store, engine: engine, ctx: ctx, cancel: cancel}
	e.GET("/health", s.live)
	e.GET("/health/live", s.live)
	e.GET("/health/ready", s.ready)

	api := e.Group("")
	api.Use(auth.Bearer(cfg.WebAuthToken, cfg.WebAuthDisabled))
	api.POST("/workflow/run", s.workflowRun)
	api.POST("/workflow/approve", s.workflowApprove)
	api.GET("/memory/:session_id", s.memory)
	return s
}

func (s *Server) Start() error {
	s.started = true
	err := (echo.StartConfig{
		Address:         s.cfg.Addr,
		HideBanner:      true,
		GracefulTimeout: 10 * time.Second,
	}).Start(s.ctx, s.echo)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.cancel()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(50 * time.Millisecond):
		return nil
	}
}

func (s *Server) live(c *echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) ready(c *echo.Context) error {
	ctx := c.Request().Context()
	if err := s.store.Ping(ctx); err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "error", "database": err.Error()})
	}
	if s.cfg.WorkspaceDir != "" {
		if info, err := os.Stat(s.cfg.WorkspaceDir); err != nil || !info.IsDir() {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "error", "workspace": "unavailable"})
		}
	}
	if s.cfg.AgentsConfigPath != "" {
		if info, err := os.Stat(s.cfg.AgentsConfigPath); err != nil || info.IsDir() {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "error", "agents_config": "unavailable"})
		}
	}
	if err := s.checkRedis(ctx); err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "error", "redis": err.Error()})
	}
	if err := s.checkLLM(ctx); err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "error", "llm": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) checkRedis(ctx context.Context) error {
	if s.cfg.RedisURL == "" {
		return nil
	}
	parsed, err := url.Parse(s.cfg.RedisURL)
	if err != nil {
		return fmt.Errorf("invalid redis url")
	}
	host := parsed.Host
	if !strings.Contains(host, ":") {
		host = net.JoinHostPort(host, "6379")
	}
	dialer := net.Dialer{Timeout: 800 * time.Millisecond}
	conn, err := dialer.DialContext(ctx, "tcp", host)
	if err != nil {
		return err
	}
	defer conn.Close()

	deadline := time.Now().Add(800 * time.Millisecond)
	_ = conn.SetDeadline(deadline)
	if _, err := conn.Write([]byte("*1\r\n$4\r\nPING\r\n")); err != nil {
		return err
	}
	buf := make([]byte, 16)
	n, err := conn.Read(buf)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(string(buf[:n]), "+PONG") {
		return fmt.Errorf("unexpected ping response")
	}
	return nil
}

func (s *Server) checkLLM(ctx context.Context) error {
	backend := strings.TrimSpace(s.cfg.DefaultLLMBackend)
	if backend == "" {
		return nil
	}

	var endpoint string
	switch backend {
	case "ollama":
		if s.cfg.OllamaBaseURL == "" {
			return fmt.Errorf("ollama base url is empty")
		}
		endpoint = strings.TrimRight(s.cfg.OllamaBaseURL, "/") + "/api/tags"
	case "llamacpp":
		if s.cfg.LlamaCPPBaseURL == "" {
			return fmt.Errorf("llamacpp base url is empty")
		}
		endpoint = strings.TrimRight(s.cfg.LlamaCPPBaseURL, "/") + "/models"
	default:
		return nil
	}

	requestCtx, cancel := context.WithTimeout(ctx, 1200*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Timeout: 1500 * time.Millisecond}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s returned %s", backend, resp.Status)
	}
	return nil
}

func (s *Server) workflowRun(c *echo.Context) error {
	var req TaskRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	req.Task = strings.TrimSpace(req.Task)
	if req.Task == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "task is required")
	}
	useCodeEngine := true
	if req.UseCodeEngine != nil {
		useCodeEngine = *req.UseCodeEngine
	}
	result, err := s.engine.RunWorkflow(c.Request().Context(), req.Task, req.SessionID, useCodeEngine)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, err.Error())
	}
	return c.JSON(http.StatusOK, result)
}

func (s *Server) workflowApprove(c *echo.Context) error {
	var req ApproveRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	req.ApprovalID = strings.TrimSpace(req.ApprovalID)
	if req.ApprovalID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "approval_id is required")
	}
	result, err := s.engine.ApproveApproval(c.Request().Context(), req.ApprovalID, strings.TrimSpace(req.Engine))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusOK, result)
}

func (s *Server) memory(c *echo.Context) error {
	limit := 50
	if raw := c.QueryParam("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err == nil && parsed > 0 && parsed <= 200 {
			limit = parsed
		}
	}
	events, err := s.store.Recall(c.Request().Context(), c.Param("session_id"), limit)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{"session_id": c.Param("session_id"), "events": events})
}
