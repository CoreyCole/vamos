package agentchat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type HTTPOpaqueSettlementReceiver struct {
	URL, Token string
	Client     *http.Client
}

func (c HTTPOpaqueSettlementReceiver) DeliverOpaqueSettlement(
	ctx context.Context,
	request OpaqueSettlementDeliveryRequest,
) error {
	body, err := json.Marshal(request)
	if err != nil {
		return err
	}
	client := c.Client
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		strings.TrimRight(c.URL, "/")+"/vamos/opaque-settlements",
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("opaque settlement delivery: %s", resp.Status)
	}
	return nil
}
