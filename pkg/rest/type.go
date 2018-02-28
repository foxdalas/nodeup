package rest

import (
	"github.com/foxdalas/nodeup/pkg/nodeup"
	"github.com/labstack/echo"
	"github.com/patrickmn/go-cache"
)

type Echo struct {
	*echo.Echo
	nodeup *nodeup.NodeUP
	cache  *cache.Cache
}

type SetupHost struct {
	Role        string `json:"role" xml:"role" form:"role" query:"role"`
	Environment string `json:"environment" xml:"environment" form:"environment" query:"environment"`
	Sensitive   string `json:"sensitive" xml:"sensitive" form:"sensitive" query:"sensitive"`
}

type WebSocketLog struct {
	Url string
}

//Action chef/stop/start...
//State:
// 0 - Done with ok
// 1 - Done with error
// 100 - In progress

type Progress struct {
	Action string `json:"action" xml:"action" form:"action" query:"action"`
	State  int    `json:"state" xml:"state" form:"state" query:"state"`
}

type SimpleResponse struct {
	Message string `json:"message" xml:"message" form:"message" query:"message"`
	Error   string `json:"error" xml:"error" form:"error" query:"error"`
}
