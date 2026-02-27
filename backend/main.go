package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/cors"
	_ "golang.org/x/image/webp"
)

var (
	runwareAPIURL    = "https://api.runware.ai/v1"
	runwareAPIKey    string
	useMock          = false
	modelRunnerURL   string
	modelRunnerModel string
)

func init() {
	loadEnvFile(".env")
	runwareAPIKey = getEnv("RUNWARE_API_KEY", "")
	modelRunnerURL = getEnv("MODEL_RUNNER_URL", "http://localhost:12434/engines/llama.cpp/v1/chat/completions")
	modelRunnerModel = getEnv("MODEL_RUNNER_MODEL", "ai/gemma3:4B-Q4_K_M")
}

func loadEnvFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			if os.Getenv(key) == "" {
				os.Setenv(key, val)
			}
		}
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// All available models
var availableModels = map[string]struct {
	Name  string
	Price float64
}{
	"google:3@3":   {Name: "Veo 3.1 Fast", Price: 0.80},
	"pixverse:1@7": {Name: "PixVerse v5.6", Price: 0.24},
	"vidu:4@2":     {Name: "Vidu Q3 Turbo", Price: 0.13},
	"vidu:4@1":     {Name: "Vidu Q3", Price: 0.05},
}

// Aspect ratio presets (720p)
var ratioSizes = map[string][2]int{
	"9:16": {720, 1280},
	"16:9": {1280, 720},
	"1:1":  {720, 720},
}

type Job struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	VideoURL  string `json:"video_url,omitempty"`
	Prompt    string `json:"prompt"`
	Model     string `json:"model"`
	Ratio     string `json:"ratio"`
	Duration  int    `json:"duration"`
	CreatedAt string `json:"created_at"`
	Error     string `json:"error,omitempty"`

	// internal, not serialized
	imagePaths []string
	modelID    string
}

var (
	jobs   = make(map[string]*Job)
	jobsMu sync.RWMutex
)

func main() {
	if runwareAPIKey == "" && !useMock {
		fmt.Println("ERROR: Set RUNWARE_API_KEY in .env")
		os.Exit(1)
	}

	os.MkdirAll("uploads", 0755)
	os.MkdirAll("videos", 0755)

	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/upload", handleUpload)
	mux.HandleFunc("POST /api/generate", handleGenerate)
	mux.HandleFunc("POST /api/auto-prompt", handleAutoPrompt)
	mux.HandleFunc("GET /api/status/{id}", handleStatus)
	mux.HandleFunc("GET /api/jobs", handleListJobs)
	mux.HandleFunc("GET /health", handleHealth)

	mux.Handle("/uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir("uploads"))))
	mux.Handle("/videos/", http.StripPrefix("/videos/", http.FileServer(http.Dir("videos"))))

	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"http://localhost:3000"},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type"},
		AllowCredentials: true,
	})

	mode := "MOCK"
	if !useMock {
		mode = "Runware AI"
	}

	fmt.Printf("\nProduct Video AI - Go Backend\n")
	fmt.Printf("  Mode:     %s\n", mode)
	fmt.Printf("  Server:   http://localhost:8080\n")
	fmt.Printf("  Frontend: http://localhost:3000\n\n")

	if err := http.ListenAndServe(":8080", c.Handler(mux)); err != nil {
		fmt.Printf("Server failed: %v\n", err)
		os.Exit(1)
	}
}

func handleUpload(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(10 << 20)

	file, header, err := r.FormFile("image")
	if err != nil {
		jsonError(w, "No image file provided", http.StatusBadRequest)
		return
	}
	defer file.Close()

	ext := filepath.Ext(header.Filename)
	allowed := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true}
	if !allowed[ext] {
		jsonError(w, "Only JPG, PNG, WEBP images are allowed", http.StatusBadRequest)
		return
	}

	filename := uuid.New().String() + ext
	savePath := filepath.Join("uploads", filename)

	dst, err := os.Create(savePath)
	if err != nil {
		jsonError(w, "Failed to save image", http.StatusInternalServerError)
		return
	}
	defer dst.Close()
	io.Copy(dst, file)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message":   "Image uploaded successfully",
		"filename":  filename,
		"image_url": fmt.Sprintf("http://localhost:8080/uploads/%s", filename),
	})
}

func handleAutoPrompt(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Filenames      []string `json:"filenames"`
		ProductName    string   `json:"product_name"`
		SceneNumber    int      `json:"scene_number"`
		TotalScenes    int      `json:"total_scenes"`
		Duration       int      `json:"duration"`
		PreviousPrompts []string `json:"previous_prompts"` // prompts from earlier scenes
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if len(req.Filenames) == 0 {
		jsonError(w, "filenames is required", http.StatusBadRequest)
		return
	}

	// Encode all images as JPEG base64
	var imageBase64s []string
	for _, fn := range req.Filenames {
		imgPath := filepath.Join("uploads", fn)
		if _, err := os.Stat(imgPath); os.IsNotExist(err) {
			continue
		}

		imgFile, err := os.Open(imgPath)
		if err != nil {
			continue
		}

		img, _, err := image.Decode(imgFile)
		imgFile.Close()
		if err != nil {
			continue
		}

		var jpegBuf bytes.Buffer
		if err := jpeg.Encode(&jpegBuf, img, &jpeg.Options{Quality: 80}); err != nil {
			continue
		}

		b64 := fmt.Sprintf("data:image/jpeg;base64,%s", base64.StdEncoding.EncodeToString(jpegBuf.Bytes()))
		imageBase64s = append(imageBase64s, b64)
		fmt.Printf("AutoPrompt: Image %s converted to JPEG (%d KB)\n", fn, jpegBuf.Len()/1024)
	}

	if len(imageBase64s) == 0 {
		jsonError(w, "No valid images found", http.StatusBadRequest)
		return
	}

	productCtx := "a product"
	if req.ProductName != "" {
		productCtx = req.ProductName
	}

	sceneNum := req.SceneNumber
	if sceneNum < 1 {
		sceneNum = 1
	}

	dur := req.Duration
	if dur < 1 {
		dur = 4
	}

	// Build context about previous scenes so the LLM knows what was already covered
	previousCtx := ""
	if len(req.PreviousPrompts) > 0 {
		previousCtx = "Previous scenes already done:\n"
		for i, p := range req.PreviousPrompts {
			previousCtx += fmt.Sprintf("  Scene %d: %s\n", i+1, p)
		}
		previousCtx += fmt.Sprintf("Now write scene %d. Do NOT repeat what previous scenes already show. Use a different camera move, angle, or setting.\n", sceneNum)
	}

	userPrompt := fmt.Sprintf(
		"Write a short video prompt for a %d-second ad scene for %s (shown in the attached images). "+
			"%s"+
			"RULES: "+
			"1-2 sentences MAXIMUM. "+
			"One camera move, one action. "+
			"Include: camera move + lighting + mood. "+
			"Do NOT describe the product appearance. "+
			"Do NOT use labels like 'Camera:', 'Lighting:', 'Scene:'. "+
			"Do NOT say the duration. "+
			"Just write the prompt as a plain sentence. "+
			"Example: 'Slow orbit around the product on marble surface. Warm rim lighting, soft bokeh. Premium feel.' "+
			"Output ONLY the prompt.",
		dur, productCtx, previousCtx,
	)

	// Build message content: text + all images
	contentParts := []map[string]interface{}{
		{
			"type": "text",
			"text": userPrompt,
		},
	}
	for _, b64 := range imageBase64s {
		contentParts = append(contentParts, map[string]interface{}{
			"type": "image_url",
			"image_url": map[string]string{
				"url": b64,
			},
		})
	}

	chatPayload := map[string]interface{}{
		"model": modelRunnerModel,
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": contentParts,
			},
		},
		"max_tokens": 300,
	}

	chatBody, _ := json.Marshal(chatPayload)

	fmt.Printf("AutoPrompt: Sending %d image(s) to %s (scene %d/%d)...\n", len(imageBase64s), modelRunnerModel, sceneNum, req.TotalScenes)

	client := &http.Client{Timeout: 60 * time.Second}
	httpReq, _ := http.NewRequest("POST", modelRunnerURL, bytes.NewBuffer(chatBody))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		jsonError(w, fmt.Sprintf("Model Runner error: %v", err), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		jsonError(w, fmt.Sprintf("Model Runner %d: %s", resp.StatusCode, string(body)), http.StatusInternalServerError)
		return
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &chatResp); err != nil || len(chatResp.Choices) == 0 {
		jsonError(w, "Failed to parse model response", http.StatusInternalServerError)
		return
	}

	prompt := chatResp.Choices[0].Message.Content
	fmt.Printf("AutoPrompt: Generated → %s\n", prompt)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"prompt": prompt,
	})
}

func handleGenerate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Filenames   []string `json:"filenames"`
		Prompt      string   `json:"prompt"`
		Model       string   `json:"model"`
		Ratio       string   `json:"ratio"`
		ProductName string   `json:"product_name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if len(req.Filenames) == 0 {
		jsonError(w, "filenames is required", http.StatusBadRequest)
		return
	}

	// Validate model
	modelInfo, ok := availableModels[req.Model]
	if !ok {
		jsonError(w, fmt.Sprintf("Unknown model: %s", req.Model), http.StatusBadRequest)
		return
	}

	// Validate images exist
	var imagePaths []string
	for _, fn := range req.Filenames {
		p := filepath.Join("uploads", fn)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			jsonError(w, fmt.Sprintf("Image not found: %s", fn), http.StatusBadRequest)
			return
		}
		imagePaths = append(imagePaths, p)
	}

	// Build prompt — use provided prompt, or a simple default
	finalPrompt := req.Prompt
	if finalPrompt == "" {
		if req.ProductName != "" {
			finalPrompt = fmt.Sprintf("Commercial advertisement for %s. Slow orbit, dramatic lighting, premium aesthetic. Sharp focus.", req.ProductName)
		} else {
			finalPrompt = "Commercial advertisement for a product. Slow orbit, dramatic lighting, premium aesthetic. Sharp focus."
		}
	}

	// Default ratio
	ratio := req.Ratio
	if _, ok := ratioSizes[ratio]; !ok {
		ratio = "9:16"
	}

	job := &Job{
		ID:         uuid.New().String()[:12],
		Status:     "processing",
		Prompt:     finalPrompt,
		Model:      modelInfo.Name,
		Ratio:      ratio,
		Duration:   4,
		CreatedAt:  time.Now().Format(time.RFC3339),
		imagePaths: imagePaths,
		modelID:    req.Model,
	}

	jobsMu.Lock()
	jobs[job.ID] = job
	jobsMu.Unlock()

	if useMock {
		go mockGenerate(job)
	} else {
		go runwareGenerate(job)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"job_id":  job.ID,
		"status":  "processing",
		"message": "Video generation started",
	})
}

func mockGenerate(job *Job) {
	time.Sleep(5 * time.Second)
	jobsMu.Lock()
	defer jobsMu.Unlock()
	job.Status = "completed"
	job.VideoURL = "https://www.w3schools.com/html/mov_bbb.mp4"
}

func runwareGenerate(job *Job) {
	fmt.Printf("Job %s: Model=%s Images=%d\n", job.ID, job.Model, len(job.imagePaths))
	fmt.Printf("Job %s: Prompt=%s\n", job.ID, job.Prompt)

	// Clamp to max 2 images (first + last)
	usePaths := job.imagePaths
	if len(usePaths) > 2 {
		usePaths = []string{usePaths[0], usePaths[len(usePaths)-1]}
		fmt.Printf("Job %s: Clamped %d images → 2 (first + last)\n", job.ID, len(job.imagePaths))
	}

	// Build frameImages
	var frameImages []map[string]interface{}
	for i, imgPath := range usePaths {
		imageData, err := os.ReadFile(imgPath)
		if err != nil {
			setJobError(job, fmt.Sprintf("Failed to read image %d: %v", i+1, err))
			return
		}

		ext := filepath.Ext(imgPath)
		mediaType := "image/jpeg"
		switch ext {
		case ".png":
			mediaType = "image/png"
		case ".webp":
			mediaType = "image/webp"
		}

		imageBase64 := fmt.Sprintf("data:%s;base64,%s", mediaType, base64.StdEncoding.EncodeToString(imageData))

		frame := map[string]interface{}{
			"inputImage": imageBase64,
		}

		if len(usePaths) == 1 {
			frame["frame"] = "first"
		} else if i == 0 {
			frame["frame"] = "first"
		} else if i == len(usePaths)-1 {
			frame["frame"] = "last"
		}

		frameImages = append(frameImages, frame)
	}

	// Get dimensions from ratio
	size := ratioSizes[job.Ratio]
	if size == [2]int{} {
		size = ratioSizes["9:16"]
	}

	taskUUID := uuid.New().String()

	payload := map[string]interface{}{
		"taskType":       "videoInference",
		"taskUUID":       taskUUID,
		"positivePrompt": job.Prompt,
		"model":          job.modelID,
		"width":          size[0],
		"height":         size[1],
		"duration":       job.Duration,
		"deliveryMethod": "async",
		"outputFormat":   "mp4",
		"numberResults":  1,
		"includeCost":    true,
		"outputQuality":  85,
		"frameImages":    frameImages,
	}

	// Model-specific provider settings
	switch {
	case strings.HasPrefix(job.modelID, "google:"):
		payload["fps"] = 24
		payload["providerSettings"] = map[string]interface{}{
			"google": map[string]interface{}{
				"generateAudio": true,
				"enhancePrompt": true,
			},
		}
	case strings.HasPrefix(job.modelID, "vidu:"):
		payload["providerSettings"] = map[string]interface{}{
			"vidu": map[string]interface{}{
				"audio": true,
			},
		}
	case strings.HasPrefix(job.modelID, "pixverse:"):
		payload["providerSettings"] = map[string]interface{}{
			"pixverse": map[string]interface{}{
				"thinking": "auto",
			},
		}
	}

	reqPayload := []map[string]interface{}{payload}
	reqBody, _ := json.Marshal(reqPayload)

	fmt.Printf("Job %s: Calling Runware (%s)...\n", job.ID, job.modelID)

	client := &http.Client{Timeout: 5 * time.Minute}
	httpReq, _ := http.NewRequest("POST", runwareAPIURL, bytes.NewBuffer(reqBody))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+runwareAPIKey)

	resp, err := client.Do(httpReq)
	if err != nil {
		setJobError(job, fmt.Sprintf("Runware API error: %v", err))
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("Job %s: Response [%d]: %s\n", job.ID, resp.StatusCode, string(body))

	if resp.StatusCode != 200 {
		setJobError(job, fmt.Sprintf("Runware API %d: %s", resp.StatusCode, string(body)))
		return
	}

	var response struct {
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		setJobError(job, fmt.Sprintf("Failed to parse response: %v", err))
		return
	}

	// Check for direct video URL
	for _, result := range response.Data {
		status, _ := result["status"].(string)
		if status == "success" {
			if videoURL, ok := result["videoURL"].(string); ok && videoURL != "" {
				completeJobWithVideo(job, videoURL)
				return
			}
		}
	}

	// Async — poll for result
	fmt.Printf("Job %s: Async, polling...\n", job.ID)
	pollResult(job, taskUUID)
}

func pollResult(job *Job, taskUUID string) {
	client := &http.Client{Timeout: 30 * time.Second}

	for i := 0; i < 120; i++ {
		time.Sleep(5 * time.Second)

		payload := []map[string]interface{}{
			{
				"taskType": "getResponse",
				"taskUUID": taskUUID,
			},
		}

		body, _ := json.Marshal(payload)
		req, _ := http.NewRequest("POST", runwareAPIURL, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+runwareAPIKey)

		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("Job %s: Poll error: %v\n", job.ID, err)
			continue
		}

		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		fmt.Printf("Job %s: Poll [%d]: %s\n", job.ID, resp.StatusCode, string(respBody))

		var pollResp struct {
			Data   []map[string]interface{} `json:"data"`
			Errors []map[string]interface{} `json:"errors"`
		}
		if err := json.Unmarshal(respBody, &pollResp); err != nil {
			continue
		}

		for _, e := range pollResp.Errors {
			if msg, ok := e["message"].(string); ok && msg != "" {
				setJobError(job, msg)
				return
			}
		}

		for _, result := range pollResp.Data {
			status, _ := result["status"].(string)

			if status == "success" {
				if videoURL, ok := result["videoURL"].(string); ok && videoURL != "" {
					completeJobWithVideo(job, videoURL)
					return
				}
			}

			if status == "error" {
				errMsg := "Unknown error"
				if msg, ok := result["message"].(string); ok {
					errMsg = msg
				}
				setJobError(job, errMsg)
				return
			}
		}
	}

	setJobError(job, "Timed out waiting for video")
}

func completeJobWithVideo(job *Job, remoteURL string) {
	fmt.Printf("Job %s: Done! Downloading %s\n", job.ID, remoteURL)

	localPath := filepath.Join("videos", job.ID+".mp4")
	localURL := fmt.Sprintf("http://localhost:8080/videos/%s.mp4", job.ID)

	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Get(remoteURL)
	if err != nil {
		fmt.Printf("Job %s: Download failed: %v, using remote URL\n", job.ID, err)
		localURL = remoteURL
	} else {
		defer resp.Body.Close()
		out, err := os.Create(localPath)
		if err != nil {
			fmt.Printf("Job %s: Save failed: %v, using remote URL\n", job.ID, err)
			localURL = remoteURL
		} else {
			written, _ := io.Copy(out, resp.Body)
			out.Close()
			fmt.Printf("Job %s: Saved %s (%d bytes)\n", job.ID, localPath, written)
		}
	}

	jobsMu.Lock()
	job.Status = "completed"
	job.VideoURL = localURL
	jobsMu.Unlock()
}

func setJobError(job *Job, errMsg string) {
	jobsMu.Lock()
	defer jobsMu.Unlock()
	job.Status = "failed"
	job.Error = errMsg
	fmt.Printf("Job %s FAILED: %s\n", job.ID, errMsg)
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	jobsMu.RLock()
	job, exists := jobs[id]
	jobsMu.RUnlock()

	if !exists {
		jsonError(w, "Job not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":        job.ID,
		"status":    job.Status,
		"video_url": job.VideoURL,
		"error":     job.Error,
	})
}

func handleListJobs(w http.ResponseWriter, r *http.Request) {
	jobsMu.RLock()
	defer jobsMu.RUnlock()

	list := make([]*Job, 0, len(jobs))
	for _, j := range jobs {
		list = append(list, j)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"mode":    map[bool]string{true: "MOCK", false: "Runware AI"}[useMock],
		"has_key": runwareAPIKey != "",
	})
}

func jsonError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
