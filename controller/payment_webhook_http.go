package controller

import (
	"errors"
	"io"
	"net/http"

	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/gin-gonic/gin"
)

const paymentWebhookBodyLimit int64 = 1 << 20

func readPaymentWebhook(c *gin.Context) ([]byte, error) {
	body := http.MaxBytesReader(c.Writer, c.Request.Body, paymentWebhookBodyLimit)
	defer body.Close()
	return io.ReadAll(body)
}

func paymentWebhookErrorStatus(err error) int {
	if errors.Is(err, model.ErrPaymentEvidenceMismatch) || errors.Is(err, model.ErrPaymentMethodMismatch) || errors.Is(err, model.ErrTopUpNotFound) || errors.Is(err, model.ErrSubscriptionOrderNotFound) {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

func paymentTerminalEventHandled(err error) bool {
	return err == nil || errors.Is(err, model.ErrTopUpNotFound) || errors.Is(err, model.ErrSubscriptionOrderNotFound) || errors.Is(err, model.ErrTopUpStatusInvalid) || errors.Is(err, model.ErrSubscriptionOrderStatusInvalid)
}
