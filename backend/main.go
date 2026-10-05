package main

import (
    "database/sql"
    "encoding/json"
    "errors"
    "fmt"
    "log"
    "net/http"
    "os"
    "path/filepath"
    "strconv"
    "strings"
    "time"

    _ "github.com/mattn/go-sqlite3"
)

type RequestRecord struct {
    ID            int64     `json:"id"`
    Filename      string    `json:"filename"`
    Status        string    `json:"status"`
    WorkflowID    string    `json:"workflow_id"`
    WorkflowURL   string    `json:"workflow_url"`
    RunID         string    `json:"run_id"`
    ErrorMessage  string    `json:"error_message,omitempty"`
    TwigTemplate  string    `json:"twig_template,omitempty"`
    CreatedAt     time.Time `json:"created_at"`
    UpdatedAt     time.Time `json:"updated_at"`
}

type createRequestPayload struct {
    Filename     string `json:"filename"`
    TwigTemplate string `json:"twig_template,omitempty"`
}

type temporaryWorkflowPayload struct {
    TwigTemplate string `json:"twig_template,omitempty"`
    TestData     map[string]any `json:"test_data,omitempty"`
}

var (
    db *sql.DB
)

func main() {
    dbPath := os.Getenv("DB_PATH")
    if dbPath == "" {
        dbPath = filepath.Join(".", "data", "requests.db")
    }

    if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
        log.Fatal(err)
    }

    if err := initDB(dbPath); err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    mux := http.NewServeMux()
    mux.HandleFunc("/health", healthHandler)
    mux.HandleFunc("/api/requests", handleRequests)
    mux.HandleFunc("/api/requests/", handleRequestByID)
    mux.HandleFunc("/api/requests/temporary-workflow", handleTemporaryWorkflow)

    port := os.Getenv("PORT")
    if port == "" {
        port = "8081"
    }

    log.Printf("Starting backend on :%s", port)
    log.Fatal(http.ListenAndServe(":"+port, withCORS(mux)))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleRequests(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodGet:
        listRequests(w, r)
    case http.MethodPost:
        createRequest(w, r)
    default:
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
    }
}

func handleRequestByID(w http.ResponseWriter, r *http.Request) {
    idStr := strings.TrimPrefix(r.URL.Path, "/api/requests/")
    if idStr == "" || strings.Contains(idStr, "/") {
        http.NotFound(w, r)
        return
    }

    id, err := strconv.ParseInt(idStr, 10, 64)
    if err != nil {
        http.Error(w, "Invalid request ID", http.StatusBadRequest)
        return
    }

    switch r.Method {
    case http.MethodGet:
        getRequest(w, r, id)
    case http.MethodPost:
        if strings.HasSuffix(r.URL.Path, "/temporary-workflow") {
            executeTemporaryWorkflowForRequest(w, r, id)
            return
        }
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
    default:
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
    }
}

func handleTemporaryWorkflow(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    var payload temporaryWorkflowPayload
    if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
        http.Error(w, "Invalid JSON body", http.StatusBadRequest)
        return
    }

    sampleData := map[string]any{"filename": "a=sample.txt", "status": "success"}
    if payload.TestData != nil {
        sampleData = payload.TestData
    }
    if payload.TwigTemplate == "" {
        payload.TwigTemplate = "{{ filename }} | {{ status }}"
    }

    result, err := triggerTemporaryWorkflow(sampleData, payload.TwigTemplate)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(result)
}

func initDB(dbPath string) error {
    var err error
    db, err = sql.Open("sqlite3", dbPath)
    if err != nil {
        return err
    }

    schema := `
    CREATE TABLE IF NOT EXISTS requests (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        filename TEXT NOT NULL,
        status TEXT NOT NULL,
        workflow_id TEXT,
        workflow_url TEXT,
        run_id TEXT,
        error_message TEXT,
        twig_template TEXT,
        created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
        updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
    );`

    _, err = db.Exec(schema)
    if err != nil {
        return err
    }

    return nil
}

func createRequest(w http.ResponseWriter, r *http.Request) {
    var payload createRequestPayload
    if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
        http.Error(w, "Invalid JSON body", http.StatusBadRequest)
        return
    }

    if strings.TrimSpace(payload.Filename) == "" {
        http.Error(w, "filename is required", http.StatusBadRequest)
        return
    }

    record := RequestRecord{
        Filename:     payload.Filename,
        Status:      "pending",
        TwigTemplate: payload.TwigTemplate,
        CreatedAt:   time.Now(),
        UpdatedAt:   time.Now(),
    }

    res, err := db.Exec(`
        INSERT INTO requests (filename, status, twig_template, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?)
    `, record.Filename, record.Status, record.TwigTemplate, record.CreatedAt, record.UpdatedAt)
    if err != nil {
        http.Error(w, fmt.Sprintf("database error: %v", err), http.StatusInternalServerError)
        return
    }

    id, _ := res.LastInsertId()
    record.ID = id

    workflowURL := os.Getenv("N8N_BASE_URL")
    if workflowURL == "" {
        workflowURL = "http://localhost:5678"
    }
    record.WorkflowURL = workflowURL + "/workflow/filename-validator"
    record.WorkflowID = "filename-validator"

    _, err = db.Exec(`UPDATE requests SET workflow_id = ?, workflow_url = ?, updated_at = ? WHERE id = ?`, record.WorkflowID, record.WorkflowURL, time.Now(), id)
    if err != nil {
        log.Printf("update workflow metadata error: %v", err)
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(record)
}

func listRequests(w http.ResponseWriter, r *http.Request) {
    rows, err := db.Query(`SELECT id, filename, status, workflow_id, workflow_url, run_id, error_message, twig_template, created_at, updated_at FROM requests ORDER BY created_at DESC`)
    if err != nil {
        http.Error(w, fmt.Sprintf("database error: %v", err), http.StatusInternalServerError)
        return
    }
    defer rows.Close()

    var items []RequestRecord
    for rows.Next() {
        var rec RequestRecord
        if err := rows.Scan(&rec.ID, &rec.Filename, &rec.Status, &rec.WorkflowID, &rec.WorkflowURL, &rec.RunID, &rec.ErrorMessage, &rec.TwigTemplate, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
            http.Error(w, fmt.Sprintf("scan error: %v", err), http.StatusInternalServerError)
            return
        }
        items = append(items, rec)
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(items)
}

func getRequest(w http.ResponseWriter, r *http.Request, id int64) {
    row := db.QueryRow(`SELECT id, filename, status, workflow_id, workflow_url, run_id, error_message, twig_template, created_at, updated_at FROM requests WHERE id = ?`, id)

    var rec RequestRecord
    if err := row.Scan(&rec.ID, &rec.Filename, &rec.Status, &rec.WorkflowID, &rec.WorkflowURL, &rec.RunID, &rec.ErrorMessage, &rec.TwigTemplate, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            http.NotFound(w, r)
            return
        }
        http.Error(w, fmt.Sprintf("scan error: %v", err), http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(rec)
}

func executeTemporaryWorkflowForRequest(w http.ResponseWriter, r *http.Request, requestID int64) {
    var payload temporaryWorkflowPayload
    if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
        http.Error(w, "Invalid JSON body", http.StatusBadRequest)
        return
    }

    row := db.QueryRow(`SELECT filename, twig_template FROM requests WHERE id = ?`, requestID)
    var filename, twig string
    if err := row.Scan(&filename, &twig); err != nil {
        http.Error(w, "Request not found", http.StatusNotFound)
        return
    }
    if payload.TwigTemplate != "" {
        twig = payload.TwigTemplate
    }
    if payload.TestData == nil {
        payload.TestData = map[string]any{"filename": filename, "status": "success", "request_id": requestID}
    }

    result, err := triggerTemporaryWorkflow(payload.TestData, twig)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    _, err = db.Exec(`UPDATE requests SET run_id = ?, status = ?, updated_at = ? WHERE id = ?`, result["run_id"], result["status"], time.Now(), requestID)
    if err != nil {
        log.Printf("update run metadata error: %v", err)
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(result)
}

func triggerTemporaryWorkflow(testData map[string]any, twigTemplate string) (map[string]any, error) {
    n8nBaseURL := os.Getenv("N8N_BASE_URL")
    if n8nBaseURL == "" {
        n8nBaseURL = "http://localhost:5678"
    }

    payload := map[string]any{
        "name": fmt.Sprintf("temp-workflow-%d", time.Now().Unix()),
        "active": false,
        "settings": map[string]any{},
        "nodes": []map[string]any{
            {
                "id": "webhook-1",
                "name": "Webhook Trigger",
                "type": "n8n-nodes-base.webhook",
                "typeVersion": 1,
                "position": []int{240, 300},
                "parameters": map[string]any{
                    "httpMethod": "POST",
                    "path": "temp-workflow-" + strconv.FormatInt(time.Now().Unix(), 10),
                    "responseMode": "lastNode",
                    "authentication": "none",
                },
            },
            {
                "id": "code-1",
                "name": "Twig / Transform",
                "type": "n8n-nodes-base.code",
                "typeVersion": 2,
                "position": []int{480, 300},
                "parameters": map[string]any{
                    "language": "python",
                    "code": fmt.Sprintf("data = _input.first().get('json', {})\nfilename = data.get('filename', '')\nstatus = data.get('status', 'success')\ntext = %q\nprint('twig-template:', text)\nreturn {'json': {'filename': filename, 'status': status, 'twig_output': text}}\n", twigTemplate),
                },
            },
            {
                "id": "response-1",
                "name": "Respond to Webhook",
                "type": "n8n-nodes-base.respondToWebhook",
                "typeVersion": 1,
                "position": []int{720, 300},
                "parameters": map[string]any{},
            },
        },
        "connections": map[string]any{
            "webhook-1": map[string]any{"main": []map[string]any{{"node": "code-1", "type": "main", "index": 0}}},
            "code-1": map[string]any{"main": []map[string]any{{"node": "response-1", "type": "main", "index": 0}}},
        },
    }

    body, err := json.Marshal(payload)
    if err != nil {
        return nil, err
    }

    req, err := http.NewRequest(http.MethodPost, n8nBaseURL+"/rest/workflows", strings.NewReader(string(body)))
    if err != nil {
        return nil, err
    }
    req.Header.Set("Content-Type", "application/json")
    if apiKey := os.Getenv("N8N_API_KEY"); apiKey != "" {
        req.Header.Set("Authorization", "Bearer "+apiKey)
    }

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("n8n workflow create error: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode >= 400 {
        return nil, fmt.Errorf("n8n workflow create failed with status %s", resp.Status)
    }

    var workflowResp map[string]any
    if err := json.NewDecoder(resp.Body).Decode(&workflowResp); err != nil {
        return nil, fmt.Errorf("decode workflow response: %w", err)
    }

    workflowID, _ := workflowResp["id"].(string)
    if workflowID == "" {
        workflowID, _ = workflowResp["id"].(float64)
    }

    runPayload := map[string]any{"json": testData}
    runBody, _ := json.Marshal(runPayload)
    runReq, err := http.NewRequest(http.MethodPost, n8nBaseURL+"/rest/workflows/"+fmt.Sprintf("%v", workflowID)+"/run", strings.NewReader(string(runBody)))
    if err != nil {
        return nil, err
    }
    runReq.Header.Set("Content-Type", "application/json")
    if apiKey := os.Getenv("N8N_API_KEY"); apiKey != "" {
        runReq.Header.Set("Authorization", "Bearer "+apiKey)
    }

    runResp, err := http.DefaultClient.Do(runReq)
    if err != nil {
        return nil, fmt.Errorf("n8n workflow run error: %w", err)
    }
    defer runResp.Body.Close()

    if runResp.StatusCode >= 400 {
        return nil, fmt.Errorf("n8n workflow run failed with status %s", runResp.Status)
    }

    var runResult map[string]any
    if err := json.NewDecoder(runResp.Body).Decode(&runResult); err != nil {
        return nil, fmt.Errorf("decode run response: %w", err)
    }

    result := map[string]any{
        "workflow_id": workflowID,
        "workflow_url": n8nBaseURL + "/workflow/" + fmt.Sprintf("%v", workflowID),
        "run_id": runResult["runId"],
        "status": "success",
    }
    return result, nil
}

func withCORS(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Access-Control-Allow-Origin", "*")
        w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
        if r.Method == http.MethodOptions {
            w.WriteHeader(http.StatusNoContent)
            return
        }
        next.ServeHTTP(w, r)
    })
}
