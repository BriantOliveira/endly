package endly_test

import (
	"github.com/stretchr/testify/assert"
	"github.com/viant/endly"
	"github.com/viant/toolbox"
	"net/http"
	"path"
	"strings"
	"testing"
)

func TestHTTPEndpointService_Run(t *testing.T) {

	parent := toolbox.CallerDirectory(3)
	var httpTripBaseDir = path.Join(parent, "test", "http", "runner", "send")
	manager := endly.NewManager()
	context := manager.NewContext(toolbox.NewContext())
	service, _ := context.Service(endly.HTTPEndpointServiceID)

	response := service.Run(context, &endly.HTTPEndpointListenRequest{
		BaseDirectory: httpTripBaseDir,
		Port:          8718,
	})
	assert.Equal(t, "", response.Error)
	listenResponse, ok := response.Response.(*endly.HTTPEndpointListenResponse)
	if assert.True(t, ok) {
		assert.Equal(t, 2, len(listenResponse.Trips))
		client := http.DefaultClient
		{
			response, err := client.Post("http://127.0.0.1:8718/send1", "", strings.NewReader("0123456789"))
			assert.Nil(t, err)
			assert.Equal(t, 200, response.StatusCode)
		}
		{
			response, err := client.Post("http://127.0.0.1:8718/send1", "", strings.NewReader("0123456789"))
			assert.Nil(t, err)
			assert.Equal(t, 200, response.StatusCode)
		}
		{
			response, err := client.Post("http://127.0.0.1:8718/send2", "", strings.NewReader("xc"))
			assert.Nil(t, err)
			assert.Equal(t, 200, response.StatusCode)
		}
	}

}

func TestHTTPEndpointService_Run_WithError(t *testing.T) {

	parent := toolbox.CallerDirectory(3)
	var httpTripBaseDir = path.Join(parent, "test", "http", "runner", "send")
	manager := endly.NewManager()
	context := manager.NewContext(toolbox.NewContext())
	service, _ := context.Service(endly.HTTPEndpointServiceID)

	{ //no port error
		response := service.Run(context, &endly.HTTPEndpointListenRequest{
			BaseDirectory: httpTripBaseDir,
		})
		assert.True(t, response.Error != "")
	}
	{ //no port error
		response := service.Run(context, &endly.HTTPEndpointListenRequest{
			Port: 1,
		})
		assert.True(t, response.Error != "")
	}

}