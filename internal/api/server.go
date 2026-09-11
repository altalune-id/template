// Package api wires the Connect-RPC handlers into an http.Handler.
package api

import (
	"net/http"

	"connectrpc.com/connect"

	authv1connect "altalune.id/template/gen/go/auth/v1/authv1connect"
	blogv1connect "altalune.id/template/gen/go/blog/v1/blogv1connect"
	todov1connect "altalune.id/template/gen/go/todo/v1/todov1connect"
	"altalune.id/template/internal/api/interceptor"
	"altalune.id/template/internal/apperror"
	"altalune.id/template/internal/auth"
	"altalune.id/template/internal/blog"
	"altalune.id/template/internal/blog/category"
	"altalune.id/template/internal/blog/tag"
	"altalune.id/template/internal/invite"
	"altalune.id/template/internal/org"
	"altalune.id/template/internal/platform"
	"altalune.id/template/internal/platform/config"
	"altalune.id/template/internal/platform/tokens"
	"altalune.id/template/internal/project"
	"altalune.id/template/internal/todo"
	"altalune.id/template/internal/user"
)

// Server holds the wired Connect handlers and their runtime configuration.
type Server struct {
	Cfg    *config.Config
	Kernel *platform.Kernel

	Auths    *auth.Service
	Users    *user.Service
	Orgs     *org.Service
	Projects *project.Service
	Todos    *todo.Service
	Invites  *invite.Service

	Posts      *blog.Service
	Categories *category.Service
	Tags       *tag.Service

	AuthSvc *AuthService
	TodoSvc *TodoService
	BlogSvc *BlogService

	OpenAPIEnabled   bool
	OpenAPIBasicAuth *BasicAuth
}

// New builds a Server from every domain service.
func New(
	cfg *config.Config,
	kernel *platform.Kernel,
	auths *auth.Service,
	users *user.Service,
	orgs *org.Service,
	projects *project.Service,
	todos *todo.Service,
	invites *invite.Service,
	todoStore todo.Store,
	posts *blog.Service,
	categories *category.Service,
	tags *tag.Service,
) *Server {
	s := &Server{
		Cfg:      cfg,
		Kernel:   kernel,
		Auths:    auths,
		Users:    users,
		Orgs:     orgs,
		Projects: projects,
		Todos:    todos,
		Invites:  invites,

		Posts:      posts,
		Categories: categories,
		Tags:       tags,

		AuthSvc: NewAuthService(orgs),
		TodoSvc: NewTodoService(todos, todoStore, projects),
		BlogSvc: NewBlogService(posts, categories, tags, projects),
	}
	if cfg != nil {
		s.OpenAPIEnabled = cfg.API.OpenAPI.Enabled
		if cfg.API.OpenAPI.RequireBasicAuth {
			s.OpenAPIBasicAuth = &BasicAuth{
				User:     cfg.API.OpenAPI.BasicAuthUser,
				Password: cfg.API.OpenAPI.BasicAuthPassword,
			}
		}
	}
	return s
}

var (
	_ authv1connect.AuthServiceHandler = (*AuthService)(nil)
	_ todov1connect.TodoServiceHandler = (*TodoService)(nil)
	_ blogv1connect.BlogServiceHandler = (*BlogService)(nil)
)

// Handler mounts the Connect handlers plus OpenAPI endpoints under basePath+"/api".
func (s *Server) Handler(basePath string) http.Handler {
	opts := s.handlerOptions()

	inner := http.NewServeMux()
	todoPath, todoHandler := todov1connect.NewTodoServiceHandler(s.TodoSvc, opts...)
	inner.Handle(todoPath, todoHandler)
	authPath, authHandler := authv1connect.NewAuthServiceHandler(s.AuthSvc, opts...)
	inner.Handle(authPath, authHandler)
	blogPath, blogHandler := blogv1connect.NewBlogServiceHandler(s.BlogSvc, opts...)
	inner.Handle(blogPath, blogHandler)

	if s.OpenAPIEnabled {
		yamlBody, jsonBody := openAPI()
		if len(yamlBody) > 0 {
			guard := openAPIGuard(s.OpenAPIBasicAuth)
			inner.Handle("/openapi.yaml", guard(openAPIHandler(yamlBody, "application/yaml")))
			inner.Handle("/openapi.json", guard(openAPIHandler(jsonBody, "application/json")))
			inner.Handle("/docs", guard(docsHandler(basePath+"/api/openapi.yaml")))
		}
	}

	mount := basePath + "/api"
	outer := http.NewServeMux()
	outer.Handle(mount+"/", http.StripPrefix(mount, inner))
	return outer
}

func (s *Server) handlerOptions() []connect.HandlerOption {
	ics := []connect.Interceptor{
		interceptor.RequestID(),
	}
	if otel, err := interceptor.OTel(nil, nil); err == nil && otel != nil {
		ics = append(ics, otel)
	}
	ics = append(ics,
		interceptor.Wrap(s.unexpected()),
		interceptor.Auth(s.verifier()),
		interceptor.Tenant(),
	)
	return []connect.HandlerOption{connect.WithInterceptors(ics...)}
}

func (s *Server) unexpected() apperror.UnexpectedFunc {
	if s.Kernel == nil || s.Kernel.Reporter == nil {
		return nil
	}
	return s.Kernel.Reporter.Unexpected
}

func (s *Server) verifier() tokens.Verifier {
	if s.Kernel == nil {
		return nil
	}
	return s.Kernel.Verifier
}
