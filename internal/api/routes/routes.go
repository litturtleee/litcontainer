package routes

import (
	"crypto/rand"
	"github.com/gin-gonic/gin"
	"litcontainer/internal/api/handlers"
	"litcontainer/internal/api/middleware"
	"litcontainer/internal/auth"
	"litcontainer/internal/daemon"
	"litcontainer/internal/logger"
	"log"
	"os"
)

func SetupRoutes(r *gin.Engine, d *daemon.Daemon) {
	r.Use(middleware.Logger())
	r.Use(middleware.RequestLogger())
	r.Use(middleware.CORS())

	containerHandler := handlers.NewContainerHandler(d)
	networkHandler := handlers.NewNetworkHandler(d)
	imageHandler := handlers.NewImageHandler(d)

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"message": "litcontainer is running",
		})
	})

	// 加载JWT密钥
	var jwtSecret []byte
	if env := os.Getenv("JWT_SECRET"); env != "" {
		jwtSecret = []byte(env)
	} else {
		jwtSecret = make([]byte, 32)
		if _, err := rand.Read(jwtSecret); err != nil {
			logger.Error("Failed to generate JWT key: %v", err)
			log.Fatal(err)
		}
	}

	// 创建处理器
	authService := auth.NewAuthService(jwtSecret)
	authHandler := handlers.NewAuthHandler(authService)
	// 认证相关路由（不需要认证）
	auth := r.Group("/api/v1/auth")
	{
		auth.POST("/login", authHandler.Login)
		auth.POST("/register", authHandler.Register)
	}

	v1 := r.Group("/api/v1")
	//v1.Use(middleware.AuthMiddleware(authService))
	{
		// 用户认证相关
		authProtected := v1.Group("/auth")
		{
			authProtected.GET("/profile", authHandler.GetProfile)
			authProtected.POST("/logout", authHandler.Logout)
		}

		containers := v1.Group("/containers")
		{
			// 为了验证先把auth的middleware去掉
			containers.POST("/create", containerHandler.CreateContainer)
			containers.POST("/:id/start", containerHandler.StartContainer)
			containers.POST("/:id/stop", containerHandler.StopContainer)
			containers.POST("/:id/kill", containerHandler.KillContainer)
			containers.POST("/:id/wait", containerHandler.WaitContainer)
			containers.DELETE("/:id", containerHandler.RemoveContainer)
			containers.GET("/list", containerHandler.ListContainers)
			containers.GET("/:id", containerHandler.InspectContainer)
			containers.GET("/:id/logs", containerHandler.LogsContainer)
		}
	}
	// 镜像相关路由
	images := v1.Group("/images")
	{
		images.POST("/export", imageHandler.ExportImage)
	}

	// 网络相关路由
	networks := v1.Group("/networks")
	{
		networks.POST("/create", networkHandler.NetworkCreate)
		networks.DELETE("/:name", networkHandler.NetworkRemove)
		networks.GET("/list", networkHandler.NetworkList)
		networks.GET("/:name", networkHandler.NetworkInspect)
	}
}
