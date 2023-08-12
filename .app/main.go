package main

import (
	models "api-gateway/models"
	routes "api-gateway/routes"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	ztm "github.com/devcoons/go-ztm"

	c "github.com/devcoons/go-fmt-colors"
	"github.com/gin-gonic/gin"
)

var APIService ztm.Service
var ServicesSts []ztm.ServicesStatus

func main() {
	runtime.GOMAXPROCS(8)
	fmt.Println(c.FmtFgBgWhiteLBlue+"[ IMS ]"+c.FmtReset, c.FmtFgBgWhiteBlue+" INFO "+c.FmtReset, c.FmtFgBgWhiteBlack+"Initializing microservice."+c.FmtReset)

	cfgfile, present := os.LookupEnv("IMSCFGFILE")

	if !present {
		wordPtr := flag.String("cfg-file", "", "Service Configuration file")
		flag.Parse()
		if wordPtr == nil || *wordPtr == "" {
			fmt.Println(c.FmtFgBgWhiteLBlue+"[ IMS ]"+c.FmtReset, c.FmtFgBgWhiteRed+" ERRN "+c.FmtReset, c.FmtFgBgWhiteBlack+"Configuration file env.variable `IMSCFGFILE` does not exist"+c.FmtReset)
			return
		}
		cfgfile = *wordPtr
	}

	if !APIService.Initialize(cfgfile) {
		fmt.Println(c.FmtFgBgWhiteLBlue+"[ IMS ]"+c.FmtReset, c.FmtFgBgWhiteRed+" ERRN "+c.FmtReset, c.FmtFgBgWhiteBlack+"Initialization failed. Exiting application.."+c.FmtReset)
		return
	}

	fmt.Println(c.FmtFgBgWhiteLBlue+"[ IMS ]"+c.FmtReset, c.FmtFgBgWhiteBlue+" INFO "+c.FmtReset, c.FmtFgBgWhiteBlack+"Models Database auto-migration"+c.FmtReset)
	models.AutoMigrate(APIService.Database)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(gin.Recovery())
	router.Use(ztm.AddUSEService(&APIService))

	router.POST("/register", routes.RoutePOSTRegister)
	router.POST("/login", routes.RoutePOSTLogin)
	router.POST("/logout", routes.RouteUserLogout)
	router.GET("/logout", routes.RouteUserLogout)
	router.DELETE("/nonce", routes.RouteDELETENonce)

	router.GET("/gateways", func(c *gin.Context) {
		claims := APIService.ValidateUserJWT(c.Request)
		if claims == nil {
			c.Data(503, "application/json", nil)
			return
		}
		c.IndentedJSON(200, APIService.Config.Gateways)

	})
	router.GET("/services/status", func(c *gin.Context) {
		c.IndentedJSON(200, ServicesSts)
	})
	router.NoRoute(RequestForwarder)
	fmt.Println("[GIN] Starting service at [0.0.0.0:8080]")
	go ServicesHealthCheck()
	router.Run("0.0.0.0:8080")
}

func RequestForwarder(c *gin.Context) {

	var requestedPath = strings.TrimRight(c.Request.URL.Path, "/")
	var requestedUrlQuery = c.Request.URL.RawQuery

	for _, nodeDetails := range APIService.Config.Services {

		m, _ := regexp.MatchString(nodeDetails.URL, requestedPath)

		if m {

			var sclaims ztm.SJWTClaims

			claims := APIService.ValidateUserJWT(c.Request)
			if claims == nil {
				sclaims.Auth = false
				sclaims.Role = 0
				sclaims.UserId = -1
				sclaims.Service = "api-gateway"
				sclaims.Hop = 5
			} else {
				sclaims.Auth = claims.Auth
				sclaims.Role = claims.Role
				sclaims.UserId = claims.UserId
				sclaims.Service = "api-gateway"
				sclaims.Hop = 5
			}

			token := APIService.SJwt.GenerateJWT(sclaims)
			client := &http.Client{}
			req, _ := http.NewRequest(c.Request.Method, nodeDetails.Host+":"+strconv.Itoa(nodeDetails.Port)+requestedPath+"?"+requestedUrlQuery, c.Request.Body)
			req.Header = c.Request.Header
			req.Header.Del("Authorization")
			req.Header.Add("Authorization", APIService.SJwt.AuthType+" "+token)
			res, errn := client.Do(req)
			if errn == nil {
				body, _ := io.ReadAll(res.Body)
				c.Data(res.StatusCode, res.Header.Get("Content-Type"), body)
			} else {
				c.Data(503, "application/json", nil)
			}
			return
		}
	}
}

func ServicesHealthCheck() {

	for {

		var tServicesSts []ztm.ServicesStatus
		for _, nodeDetails := range APIService.Config.Services {
			var sclaims ztm.SJWTClaims
			sclaims.Auth = false
			sclaims.Role = 0
			sclaims.UserId = -1
			sclaims.Service = "api-gateway"
			sclaims.Hop = 1
			token := APIService.SJwt.GenerateJWT(sclaims)
			tServicesSts = append(tServicesSts, ztm.ServicesStatus{Name: nodeDetails.Name, IsAlive: ServiceHealthPing(nodeDetails.Host+":"+strconv.Itoa(nodeDetails.Port)+"/", token)})
		}
		ServicesSts = tServicesSts
		time.Sleep(2 * time.Second)
	}
}

func ServiceHealthPing(url string, token string) bool {
	client := &http.Client{}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Del("Authorization")
	req.Header.Add("Authorization", APIService.SJwt.AuthType+" "+token)
	req.Body = nil
	_, errn := client.Do(req)
	return errn == nil
}
