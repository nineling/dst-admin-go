package router

import (
	"dst-admin-go/middleware"
	"github.com/gin-gonic/gin"
)

func NewRoute() *gin.Engine {

	app := gin.Default()

	app.Use(middleware.Recover)
	app.Use(middleware.ShellInjectionInterceptor())
	app.Use(middleware.Authentication())

	// app.Use(middleware.CheckDstHandler())

	app.GET("/hello", func(ctx *gin.Context) {
		ctx.String(200, "Hello! Dont starve together 1.1.9.2 20230816")
	})
	router := app.Group("")
	initBackupRouter(router)
	initClusterRouter(router)
	initDstConfigRouter(router)
	initFirstRouter(router)
	InitGameRouter(router)
	initLoginRouter(router)
	initModRouter(router)
	initPlayerRouter(router)
	initProxyRouter(router)
	initStatisticsRouter(router)
	initThirdPartyRouter(router)
	initWsRouter(router)
	initSteamRouter(router)
	initTimedTaskRouter(router)

	initAutoCheck(router)
	initLevel(router)
	initWebLinkRouter(router)
	initWebhookRouter(router)

	initStaticFile(app)

	return app
}
