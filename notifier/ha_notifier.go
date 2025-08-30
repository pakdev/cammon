package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type HANotificationPayload struct {
	Message string            `json:"message"`
	Title   string            `json:"title,omitempty"`
	Data    map[string]interface{} `json:"data,omitempty"`
}

type HAWebhookPayload struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Title   string `json:"title,omitempty"`
	Objects []Object `json:"objects,omitempty"`
	Timestamp string `json:"timestamp"`
}

func sendHANotification(detection Detection, timestamp time.Time, config Config) error {
	// Create a descriptive message
	objectLabels := make([]string, len(detection.Objects))
	for i, obj := range detection.Objects {
		objectLabels[i] = fmt.Sprintf("%s (%.2f)", obj.Label, obj.Confidence)
	}

	title := "Security Alert: Motion Detected"
	message := fmt.Sprintf("Detected %d object(s): %s at %s",
		len(detection.Objects),
		strings.Join(objectLabels, ", "),
		timestamp.Format("15:04:05"))

	// If it's a webhook URL, send as webhook payload
	if strings.Contains(config.HAWebhookURL, "/api/webhook/") {
		return sendWebhookNotification(detection, timestamp, config, title, message)
	}

	// Otherwise, send as companion app notification
	return sendCompanionAppNotification(detection, config, title, message)
}

func sendWebhookNotification(detection Detection, timestamp time.Time, config Config, title, message string) error {
	payload := HAWebhookPayload{
		Type:      "detection_alert",
		Message:   message,
		Title:     title,
		Objects:   detection.Objects,
		Timestamp: timestamp.Format(time.RFC3339),
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	req, err := http.NewRequest("POST", config.HAWebhookURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook request failed with status %d", resp.StatusCode)
	}

	return nil
}

func sendCompanionAppNotification(detection Detection, config Config, title, message string) error {
	// Create notification data with additional context
	data := map[string]interface{}{
		"channel":    "security",
		"importance": "high",
		"tag":        "detection_alert",
		"group":      "security_alerts",
		"actions": []map[string]string{
			{
				"action": "view_camera",
				"title":  "View Camera",
			},
		},
		"image": "/local/camera_snapshot.jpg", // This would need to be updated based on your setup
	}

	payload := HANotificationPayload{
		Message: message,
		Title:   title,
		Data:    data,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal notification payload: %w", err)
	}

	req, err := http.NewRequest("POST", config.HAWebhookURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create notification request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if config.HAToken != "" {
		req.Header.Set("Authorization", "Bearer "+config.HAToken)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send companion app notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("companion app notification failed with status %d", resp.StatusCode)
	}

	return nil
}