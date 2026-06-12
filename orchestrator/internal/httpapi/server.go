package httpapi

import (
	"context"
	"net/http"
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
	ApproveAgentTask(ctx context.Context, sessionID, agentName, task, engine string) (codeengine.Result, error)
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
	SessionID string `json:"session_id"`
	Agent     string `json:"agent"`
	Task      string `json:"task"`
	Engine    string `json:"engine"`
}

func NewServer(cfg config.Config, store MemoryStore, engine WorkflowEngine) *Server {
	e := echo.New()
	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())
	e.Use(middleware.RequestLogger())

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
	return c.JSON(http.StatusOK, map[string]string{"status": "ready"})
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
	req.SessionID = strings.TrimSpace(req.SessionID)
	req.Agent = strings.TrimSpace(req.Agent)
	req.Task = strings.TrimSpace(req.Task)
	if req.SessionID == "" || req.Agent == "" || req.Task == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "session_id, agent and task are required")
	}
	result, err := s.engine.ApproveAgentTask(c.Request().Context(), req.SessionID, req.Agent, req.Task, strings.TrimSpace(req.Engine))
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
