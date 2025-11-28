package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Define a struct for a single history/active entry.
type DelayEntry struct {
	ID             int        `json:"id"`
	Name           string     `json:"name"`
	DateTimeAdded  time.Time  `json:"dateTimeAdded"`
	TotalDelaySecs int        `json:"totalDelaySeconds"`
	IsCompleted    bool       `json:"isCompleted"`
	CompletedTime  *time.Time `json:"completedTime,omitempty"`
}

// Struct specifically for API responses that include dynamic status fields
type ApiStatusResponse struct {
	ID                int        `json:"id"`
	Name              string     `json:"name"`
	DateTimeAdded     time.Time  `json:"dateTimeAdded"`
	TotalDelaySecs    int        `json:"totalDelaySeconds"`
	Status            string     `json:"status"`
	ElapsedTimeSecs   int        `json:"elapsedTimeSeconds"`
	RemainingTimeSecs int        `json:"remainingTimeSeconds"`
	CompletedTime     *time.Time `json:"completedTime,omitempty"`
}

// Global variables to manage the shared state.
var (
	mu           sync.Mutex
	history      = make(map[int]DelayEntry)
	historyOrder []int // keep IDs in insertion order
	nextEntryID  = 1
	maxHistory   = 10
)

const timeFormat = "2006-01-02 15:04:05 MST"

// --- Helper Functions (Navigation & Status Calculation) ---

const navBarHTML = `
<style>
    body { font-family: sans-serif; margin: 0; padding: 0; }
    nav { background-color: #333; color: white; padding: 10px; margin-bottom: 20px; }
    nav a { color: white; margin-right: 15px; text-decoration: none; }
    nav a:hover { text-decoration: underline; }
    .content { padding: 0 20px; }
    table { border-collapse: collapse; width: 100%; margin-top: 10px; }
    th, td { border: 1px solid #ddd; padding: 8px; text-align: left; }
    tr:nth-child(even) { background-color: #f2f2f2; }
    .status-complete { color: green; font-weight: bold; }
    .status-progress { color: orange; font-weight: bold; }

    /* Styles for form alignment */
    .form-group {
        display: flex;
        align-items: center;
        margin-bottom: 10px;
    }
    .form-group label {
        flex-basis: 200px; /* Fixed width for labels */
        margin-right: 20px;
        text-align: right;
    }
    .form-group input {
        flex-grow: 1; /* Inputs take remaining space */
        padding: 5px;
        max-width: 300px;
    }
</style>
<nav>
    <a href="/">Home (/)</a>
    <a href="/start">Start Timer (/start)</a>
    <a href="/status">View Statuses (/status)</a>
    | API:
    <a href="/api/status">All Statuses</a>
</nav>
<div class="content">
`
const navBarEndHTML = `</div>`

func writeResponseWithNav(w http.ResponseWriter, content string) {
	fmt.Fprint(w, navBarHTML)
	fmt.Fprint(w, content)
	fmt.Fprint(w, navBarEndHTML)
}

func getStatusDetails(entry DelayEntry) (elapsedSecs int, currentStatus, statusClass string) {
	expectedCompletionTime := entry.DateTimeAdded.Add(time.Duration(entry.TotalDelaySecs) * time.Second)
	elapsedDuration := time.Since(entry.DateTimeAdded)
	elapsedSecs = int(elapsedDuration.Seconds())
	currentStatus = "in-progress"
	statusClass = "status-progress"

	if entry.IsCompleted || time.Now().After(expectedCompletionTime) {
		currentStatus = "completed"
		statusClass = "status-complete"
		elapsedSecs = entry.TotalDelaySecs
	}
	return elapsedSecs, currentStatus, statusClass
}

// keepHistoryBounded enforces maxHistory by deleting the oldest entries.
func keepHistoryBounded() {
	if len(historyOrder) <= maxHistory {
		return
	}
	for len(historyOrder) > maxHistory {
		oldestID := historyOrder[0]
		historyOrder = historyOrder[1:]
		delete(history, oldestID)
	}
}

// parseJobIDFromPath extracts the trailing numeric ID from a path like "/status/123".
func parseJobIDFromPath(path string, prefix string) (int, error) {
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(path, "/")
	// Expect ["", prefix, "{id}"]
	if len(parts) != 3 || parts[1] != prefix {
		return 0, fmt.Errorf("invalid path")
	}
	return strconv.Atoi(parts[2])
}

// parseAPIJobIDFromPath extracts ID from a path like "/api/status/123".
func parseAPIJobIDFromPath(path string) (int, error) {
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(path, "/")
	// Expect ["", "api", "status", "{id}"]
	if len(parts) != 4 || parts[1] != "api" || parts[2] != "status" {
		return 0, fmt.Errorf("invalid path")
	}
	return strconv.Atoi(parts[3])
}

// --- Standard HTML Handlers ---

func indexHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	mu.Lock()
	defer mu.Unlock()

	content := "<h1>Countdown As A Service</h1>"
	content += "<h2>Countdown History (Last 10)</h2>"

	if len(historyOrder) == 0 {
		content += "<p>No delays recorded yet.</p>"
		writeResponseWithNav(w, content)
		return
	}

	content += `<table>
		<tr>
			<th>ID</th>
			<th>Name</th>
			<th>Date/Time Added</th>
			<th>Expected Completion Time</th>
			<th>Completed Time</th>
			<th>Total Delay (Secs)</th>
			<th>Elapsed Time (Secs)</th>
			<th>Current Status</th>
		</tr>`

	// Use the last up-to-maxHistory entries from historyOrder
	start := 0
	if len(historyOrder) > maxHistory {
		start = len(historyOrder) - maxHistory
	}
	for _, id := range historyOrder[start:] {
		entry, ok := history[id]
		if !ok {
			continue
		}

		elapsedSecs, currentStatus, statusClass := getStatusDetails(entry)
		expectedCompletionTime := entry.DateTimeAdded.Add(time.Duration(entry.TotalDelaySecs) * time.Second)

		addedTimeStr := entry.DateTimeAdded.Format(timeFormat)
		completeTimeStr := expectedCompletionTime.Format(timeFormat)

		completedTimeStr := ""
		if entry.CompletedTime != nil {
			completedTimeStr = entry.CompletedTime.Format(timeFormat)
		} else {
			completedTimeStr = "-"
		}

		idLink := fmt.Sprintf("<a href=\"/status/%d\">%d</a>", entry.ID, entry.ID)

		content += fmt.Sprintf(`
			<tr>
				<td>%s</td>
				<td>%s</td>
				<td>%s</td>
				<td>%s</td>
				<td>%s</td>
				<td>%d</td>
				<td>%d</td>
				<td class="%s">%s</td>
			</tr>`, idLink, entry.Name, addedTimeStr, completeTimeStr, completedTimeStr,
			entry.TotalDelaySecs, elapsedSecs, statusClass, currentStatus)
	}
	content += "</table>"

	writeResponseWithNav(w, content)
}

func startHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		const formHTML = `
		<h1>Add A Countdown</h1>
		<p>Use the form below to activate a new, independent delayed rule.</p>
		<form method="POST" action="/start">
            <div class="form-group">
                <label for="name">Countdown Job Name:</label>
			    <input type="text" id="name" name="name" required>
            </div>
            <div class="form-group">
                <label for="delay">Countdown Delay (in secs):</label>
			    <input type="number" id="delay" name="delay" required min="1">
            </div>
			<button type="submit">Activate Rule</button>
		</form>`
		writeResponseWithNav(w, formHTML)

	case http.MethodPost:
		delayStr := r.FormValue("delay")
		jobName := r.FormValue("name")
		delay, err := strconv.Atoi(delayStr)
		if err != nil || delay < 1 {
			http.Error(w, "Invalid delay value", http.StatusBadRequest)
			return
		}

		mu.Lock()
		newEntry := DelayEntry{
			ID:             nextEntryID,
			Name:           jobName,
			DateTimeAdded:  time.Now(),
			TotalDelaySecs: delay,
			IsCompleted:    false,
			CompletedTime:  nil,
		}
		history[nextEntryID] = newEntry
		historyOrder = append(historyOrder, nextEntryID)
		keepHistoryBounded()

		jobID := nextEntryID
		nextEntryID++
		mu.Unlock()

		log.Printf("Timer Job ID %d created. Delay: %d seconds, Name: %q\n", jobID, delay, jobName)

		go runTimer(jobID, delay)
		http.Redirect(w, r, "/", http.StatusSeeOther)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func statusIndexHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	mu.Lock()
	defer mu.Unlock()

	content := "<h1>Active Countdown Status</h1>"
	activeCount := 0

	for _, entry := range history {
		if !entry.IsCompleted {
			activeCount++
			elapsedDuration := time.Since(entry.DateTimeAdded)
			remaining := time.Duration(entry.TotalDelaySecs)*time.Second - elapsedDuration

			if remaining > 0 {
				link := fmt.Sprintf("/status/%d", entry.ID)
				content += fmt.Sprintf(
					"<p><a href=\"%s\"><strong>%d - %s</strong></a> - in-progress, remaining time %.0f seconds</p>",
					link, entry.ID, entry.Name, remaining.Seconds(),
				)
			}
		}
	}

	if activeCount == 0 && len(history) > 0 {
		content += "<p>All queued tasks are completed.</p>"
	} else if len(history) == 0 {
		content += "<p>No countdowns activated.</p>"
	}

	writeResponseWithNav(w, content)
}

func statusDetailHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	jobID, err := parseJobIDFromPath(r.URL.Path, "status")
	if err != nil {
		http.Error(w, "Invalid request URL format. Use /status/<ID>", http.StatusBadRequest)
		return
	}

	mu.Lock()
	entry, ok := history[jobID]
	mu.Unlock()

	if !ok {
		http.Error(w, fmt.Sprintf("Job ID %d not found.", jobID), http.StatusNotFound)
		return
	}

	elapsedSecs, currentStatus, _ := getStatusDetails(entry)
	remainingSecs := entry.TotalDelaySecs - elapsedSecs
	if remainingSecs < 0 {
		remainingSecs = 0
	}

	content := fmt.Sprintf("<h1>Status for Job ID: %d (%s)</h1>", jobID, entry.Name)
	content += fmt.Sprintf("<p>Status: <strong>%s</strong></p>", currentStatus)
	if currentStatus == "in-progress" {
		content += fmt.Sprintf("<p>Remaining Time: %d seconds</p>", remainingSecs)
	}
	content += fmt.Sprintf("<p>Total Delay Requested: %d seconds</p>", entry.TotalDelaySecs)
	content += fmt.Sprintf("<p>Time Added: %s</p>", entry.DateTimeAdded.Format(timeFormat))

	if entry.CompletedTime != nil {
		content += fmt.Sprintf("<p>Completed Time: %s</p>", entry.CompletedTime.Format(timeFormat))
	}

	writeResponseWithNav(w, content)
}

// --- API Handlers ---

func apiStatusIndexHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	mu.Lock()
	defer mu.Unlock()

	responseList := []ApiStatusResponse{}

	for _, entry := range history {
		elapsedSecs, currentStatus, _ := getStatusDetails(entry)
		remainingSecs := entry.TotalDelaySecs - elapsedSecs
		if remainingSecs < 0 {
			remainingSecs = 0
		}

		apiResponse := ApiStatusResponse{
			ID:                entry.ID,
			Name:              entry.Name,
			DateTimeAdded:     entry.DateTimeAdded,
			TotalDelaySecs:    entry.TotalDelaySecs,
			Status:            currentStatus,
			ElapsedTimeSecs:   elapsedSecs,
			RemainingTimeSecs: remainingSecs,
			CompletedTime:     entry.CompletedTime,
		}
		responseList = append(responseList, apiResponse)
	}

	if err := json.NewEncoder(w).Encode(responseList); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func apiStatusDetailHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	jobID, err := parseAPIJobIDFromPath(r.URL.Path)
	if err != nil {
		http.Error(w, "Invalid request URL format. Use /api/status/<ID>", http.StatusBadRequest)
		return
	}

	mu.Lock()
	entry, ok := history[jobID]
	mu.Unlock()

	if !ok {
		http.Error(w, fmt.Sprintf("Job ID %d not found.", jobID), http.StatusNotFound)
		return
	}

	elapsedSecs, currentStatus, _ := getStatusDetails(entry)
	remainingSecs := entry.TotalDelaySecs - elapsedSecs
	if remainingSecs < 0 {
		remainingSecs = 0
	}

	apiResponse := ApiStatusResponse{
		ID:                entry.ID,
		Name:              entry.Name,
		DateTimeAdded:     entry.DateTimeAdded,
		TotalDelaySecs:    entry.TotalDelaySecs,
		Status:            currentStatus,
		ElapsedTimeSecs:   elapsedSecs,
		RemainingTimeSecs: remainingSecs,
		CompletedTime:     entry.CompletedTime,
	}

	if err := json.NewEncoder(w).Encode(apiResponse); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// --- runTimer Function & Main execution ---

func runTimer(jobID int, delay int) {
	time.Sleep(time.Duration(delay) * time.Second)

	mu.Lock()
	defer mu.Unlock()

	if entry, ok := history[jobID]; ok {
		entry.IsCompleted = true
		now := time.Now()
		entry.CompletedTime = &now
		history[jobID] = entry
		log.Printf("Timer Job ID %d completed at %s.\n", jobID, now.Format(timeFormat))
	}
}

func main() {
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/start", startHandler)

	http.HandleFunc("/status/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSuffix(r.URL.Path, "/")
		if path == "/status" {
			statusIndexHandler(w, r)
		} else {
			statusDetailHandler(w, r)
		}
	})

	http.HandleFunc("/api/status/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSuffix(r.URL.Path, "/")
		if path == "/api/status" {
			apiStatusIndexHandler(w, r)
		} else {
			apiStatusDetailHandler(w, r)
		}
	})

	fmt.Println("Server starting on http://localhost:8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}

