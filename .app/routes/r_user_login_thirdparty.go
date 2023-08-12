package routes

import (
	"net/http"

	ztm "github.com/devcoons/go-ztm"
	"github.com/gin-gonic/gin"
)

func RouteGETThirdParty(c *gin.Context) {

	srv, ok := c.MustGet("service").(*ztm.Service)

	if !ok || srv == nil {
		c.IndentedJSON(http.StatusExpectationFailed, nil)
		return
	}

}
