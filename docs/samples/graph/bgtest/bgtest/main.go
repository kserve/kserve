package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

type Instance struct {
	Name   string `json:"name"`
	IntVal int    `json:"intval"`
	StrVal string `json:"strval"`
}

type Response struct {
	Source    string     `json:"from"`
	Instances []Instance `json:"instances"`
}

func main() {
	defaultURL := fmt.Sprintf("/v1/models/%s:predict", os.Getenv("target"))
	r := gin.Default()
	r.POST("/splitter", handle)
	r.POST("/switch", handle)
	r.POST("/single", handle)
	r.POST("/ensemble", handle)
	r.POST(defaultURL, handle)
	r.Run(":8080")
}

func handle(ctx *gin.Context) {
	var r Response
	ctx.ShouldBindBodyWith(&r, binding.JSON)
	fmt.Printf("req = %v\n", r)
	r.Source = os.Getenv("target")
	fmt.Printf("res = %v\n", r)
	ctx.JSON(http.StatusOK, r)
}
