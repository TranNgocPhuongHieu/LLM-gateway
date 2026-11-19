package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/TranNgocPhuongHieu/LLM-gateway/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/controller"
	"github.com/TranNgocPhuongHieu/LLM-gateway/model"
	relaycommon "github.com/TranNgocPhuongHieu/LLM-gateway/relay/common"
	"github.com/TranNgocPhuongHieu/LLM-gateway/service"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestDockerRolloutFixture is a mock-only process for the isolated Compose
// rehearsal. It shares the real admission, BillingSession and shutdown code,
// but deliberately does NOT start production migrations or payment pollers.
func TestDockerRolloutFixture(t *testing.T) {
	if os.Getenv("LLM_GATEWAY_ROLLOUT_FIXTURE") != "isolated-mock-only" {
		t.Skip("Docker rehearsal fixture only")
	}
	if os.Getenv("PGHOST") != "postgres" || os.Getenv("REDIS_CONN_STRING") != "redis://redis:6379" {
		t.Fatal("fixture requires isolated Compose service endpoints")
	}
	db, err := gorm.Open(postgres.Open("host=postgres user=fixture password=fixture dbname=fixture sslmode=disable"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	model.DB, model.LOG_DB = db, db
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatal(err)
	}
	user := model.User{Id: 1, Username: "rollout-fixture", AffCode: "rollout-fixture", Quota: 10000}
	if err := db.Where("id = 1").FirstOrCreate(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := common.InitRedisClient(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	locked, err := common.RDB.SetNX(ctx, "mock-worker-lock", os.Getenv("MOCK_REV"), 0).Result()
	if err != nil || !locked {
		t.Fatalf("duplicate worker: locked=%v err=%v", locked, err)
	}
	workerCtx, stop := context.WithCancel(ctx)
	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		<-workerCtx.Done()
		time.Sleep(500 * time.Millisecond) // finish a mock in-flight worker write
		common.RDB.Incr(ctx, "mock-worker-stopped")
		common.RDB.Del(ctx, "mock-worker-lock")
	}()
	r := gin.New()
	r.Use(controller.DrainMiddleware())
	r.GET("/healthz", controller.Health)
	r.GET("/readyz", controller.Ready)
	r.POST("/internal/drain", controller.Drain)
	auth := func(c *gin.Context) {
		if c.GetHeader("Authorization") != "Bearer local-test-key" {
			c.AbortWithStatus(401)
			return
		}
		c.Next()
	}
	r.GET("/api/usage/token/", auth, func(c *gin.Context) {
		var u model.User
		if err := db.First(&u, 1).Error; err != nil {
			c.AbortWithStatus(500)
			return
		}
		c.JSON(200, gin.H{"data": gin.H{"total_used": 10000 - u.Quota, "total_available": u.Quota}})
	})
	r.POST("/v1/chat/completions", auth, func(c *gin.Context) {
		info := &relaycommon.RelayInfo{UserId: 1, IsPlayground: true}
		session, apiErr := service.NewBillingSession(c, info, 100)
		if apiErr != nil {
			c.AbortWithStatus(500)
			return
		}
		c.Header("Content-Type", "text/event-stream")
		fmt.Fprint(c.Writer, "data: {\"choices\":[{\"delta\":{\"content\":\"OK\"}}]}\n\n")
		c.Writer.Flush()
		time.Sleep(2 * time.Second)
		if err := session.Settle(140); err != nil {
			t.Error(err)
			return
		}
		if err := session.Settle(140); err != nil {
			t.Error(err)
			return
		}
		fmt.Fprint(c.Writer, "data: [DONE]\n\n")
	})
	r.POST("/refund", auth, func(c *gin.Context) {
		session, e := service.NewBillingSession(c, &relaycommon.RelayInfo{UserId: 1, IsPlayground: true}, 100)
		if e != nil {
			c.AbortWithStatus(500)
			return
		}
		refundContext := c.Copy()
		common.GoFinancial(func() {
			time.Sleep(700 * time.Millisecond)
			session.Refund(refundContext)
			session.Refund(refundContext)
		})
		c.Status(202)
	})
	r.POST("/api/payment/sepay/webhook", auth, func(c *gin.Context) {
		time.Sleep(300 * time.Millisecond)
		common.RDB.Incr(ctx, "mock-webhook-delivered")
		c.Status(200)
	})
	srv := &http.Server{Addr: ":3000", Handler: r}
	controller.ServingReady.Store(true)
	go srv.ListenAndServe()
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	<-quit
	controller.ServingReady.Store(false)
	shutdownGateway(srv, stop, &workers)
	var final model.User
	db.First(&final, 1)
	fmt.Printf("rehearsal clean exit: revision=%s quota=%d\n", os.Getenv("MOCK_REV"), final.Quota)
}
