package codex

import (
	"encoding/json"
	"testing"
)

func TestThreadItemUnmarshalAcceptsStructuredResultFromResume(t *testing.T) {
	payload := []byte(`{
		"thread": {
			"id": "thread-1",
			"turns": [{
				"id": "turn-1",
				"status": "completed",
				"items": [{
					"type": "mcpToolCall",
					"result": {
						"_meta": null,
						"content": [{
							"type": "text",
							"text": "{\"workflow_runs\": []}"
						}],
						"structuredContent": {
							"workflow_runs": []
						}
					}
				}]
			}]
		}
	}`)
	var resume ThreadResumeResponse
	if err := json.Unmarshal(payload, &resume); err != nil {
		t.Fatalf("unmarshal thread resume response: %v", err)
	}

	got := resume.Thread.Turns[0].Items[0].Result
	want := `{"_meta":null,"content":[{"type":"text","text":"{\"workflow_runs\": []}"}],"structuredContent":{"workflow_runs":[]}}`
	if got != want {
		t.Fatalf("structured result = %q, want %q", got, want)
	}
}

func TestThreadItemUnmarshalAcceptsStringResult(t *testing.T) {
	payload := []byte(`{"type":"imageGeneration","result":"C:\\tmp\\image.png"}`)
	var item ThreadItem
	if err := json.Unmarshal(payload, &item); err != nil {
		t.Fatalf("unmarshal thread item: %v", err)
	}

	if item.Result != `C:\tmp\image.png` {
		t.Fatalf("string result = %q", item.Result)
	}
}
