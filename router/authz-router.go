package router

import (
	"github.com/TranNgocPhuongHieu/LLM-gateway/controller"
	"github.com/TranNgocPhuongHieu/LLM-gateway/middleware"

	"github.com/gin-gonic/gin"
)

// registerAuthzRoutes mounts the authorization API under its own /authz
// namespace. GET /authz/catalog returns the permission schema (resources,
// actions, and role baselines) used by the client permission editor.
func registerAuthzRoutes(apiRouter *gin.RouterGroup) {
	authzRoute := apiRouter.Group("/authz")
	authzRoute.Use(middleware.RootAuth())
	{
		authzRoute.GET("/catalog", controller.GetPermissionCatalog)
	}
}
