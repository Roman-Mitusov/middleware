package http

import (
	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"
)

type ReverseProxy struct {
	// Prepare request for further processing by proxy before sending to upstream endpoint
	PrepareRequest func(request *fiber.Ctx) error
	// Modify response before sending to upstream endpoint
	ModifyResponse func(c *fiber.Ctx) error
	// Handle error occurred during request forwarding to upstream
	HandleError func(c *fiber.Ctx) error
}

func (p *ReverseProxy) Proxy(ctx *fiber.Ctx) (err error) {

	var client = fasthttp.Client{
		NoDefaultUserAgentHeader: true,
		DisablePathNormalizing:   true,
	}

	req := ctx.Request()
	res := ctx.Response()

	if p.PrepareRequest != nil {
		if err = p.PrepareRequest(ctx); err != nil {
			return err
		}
	}

	req.Header.Del(fiber.HeaderConnection)

	if err = client.Do(req, res); err != nil {
		if p.HandleError != nil {
			if err = p.HandleError(ctx); err != nil {
				return err
			}
		}
		return err
	}

	res.Header.Del(fiber.HeaderConnection)

	if p.ModifyResponse != nil {
		if err = p.ModifyResponse(ctx); err != nil {
			return err
		}
	}

	return nil

}
