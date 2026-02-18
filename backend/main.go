package main

import (
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
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/cors"
	_ "golang.org/x/image/webp"
)

var (
	runwareAPIURL    = "https://api.runware.ai/v1"
	runwareAPIKey    = "UqL2n0wIu59mvflZRhGg09MYvXWnVasC"
	useMock          = false
	modelRunnerURL   = "http://localhost:12434/engines/llama.cpp/v1/chat/completions"
	modelRunnerModel = "ai/gemma3:4B-Q4_K_M"
)

// Each style maps to: best model + prompt + price
type StyleConfig struct {
	Model     string  // Runware model ID
	ModelName string  // Display name
	Prompt    string  // Base prompt
	Price     float64
}

var styleConfigs = map[string]StyleConfig{
	"cinematic": {
		Model:     "google:3@3",
		ModelName: "Veo 3.1 Fast",
		Prompt:    "Cinematic product commercial. Camera slowly orbits around the product. Dramatic studio lighting with soft rim light and shadows. Shallow depth of field. Slow smooth dolly movement. High-end luxury brand advertisement quality.",
		Price:     0.80,
	},
	"rotating": {
		Model:     "vidu:4@2",
		ModelName: "Vidu Q3 Turbo",
		Prompt:    "Product rotating 360 degrees on a turntable. Soft even lighting from all sides, no harsh shadows. The product spins slowly and smoothly in a complete rotation. Professional e-commerce product photography style.",
		Price:     0.13,
	},
	"lifestyle": {
		Model:     "pixverse:1@7",
		ModelName: "PixVerse v5.6",
		Prompt:    "Lifestyle product video. The product in a real-world environment. Warm golden hour natural lighting, soft bokeh background. A hand gently picks up and interacts with the product. Warm color grading, Instagram aesthetic. Authentic and relatable.",
		Price:     0.24,
	},
	"tiktok": {
		Model:     "vidu:4@1",
		ModelName: "Vidu Q3",
		Prompt:    "Viral TikTok product ad. Quick dynamic camera zoom into the product, punchy energy. The product appears with motion — sliding into frame, spinning, or dropping onto a surface with impact. Trendy Gen-Z aesthetic, high contrast, fast-paced rhythm.",
		Price:     0.05,
	},
	"unboxing": {
		Model:     "vidu:4@2",
		ModelName: "Vidu Q3 Turbo",
		Prompt:    "POV first-person unboxing video. Start with a closed premium box on a table. Two hands slowly lift the lid off the box, revealing the product nestled inside. The product is revealed at the end, not shown at the beginning. Smooth slow motion, soft natural lighting, ASMR aesthetic, satisfying reveal moment.",
		Price:     0.13,
	},
	"minimal": {
		Model:     "vidu:4@1",
		ModelName: "Vidu Q3",
		Prompt:    "Minimal clean product video. The product rests on a smooth surface. Soft directional lighting creates gentle shadows. Very subtle slow camera drift. No distractions, just the product. Apple-style minimalism.",
		Price:     0.05,
	},
}

// Aspect ratio presets (must match Vidu/PixVerse supported dimensions)
var ratioSizes = map[string][2]int{
	"9:16":  {1080, 1920}, // Mobile / TikTok / Reels
	"16:9":  {1920, 1080}, // Desktop / YouTube
	"1:1":   {1080, 1080}, // Square / Instagram
}

type Job struct {
	ID         string   `json:"id"`
	Status     string   `json:"status"`
	ImagePaths []string `json:"image_paths"`
	VideoURL   string   `json:"video_url,omitempty"`
	Prompt     string   `json:"prompt"`
	Style      string   `json:"style"`
	Ratio      string   `json:"ratio"`
	Duration   int      `json:"duration"`
	Model      string   `json:"model"`
	CreatedAt  string   `json:"created_at"`
	Error      string   `json:"error,omitempty"`
}

var (
	jobs   = make(map[string]*Job)
	jobsMu sync.RWMutex
)

func main() {
	if runwareAPIKey == "" && !useMock {
		fmt.Println("ERROR: Set runwareAPIKey in main.go")
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
	mux.HandleFunc("GET /api/models", handleListModels)
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
		Filename    string `json:"filename"`
		Style       string `json:"style"`
		Duration    int    `json:"duration"`
		ProductName string `json:"product_name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Filename == "" {
		jsonError(w, "filename is required", http.StatusBadRequest)
		return
	}

	imagePath := filepath.Join("uploads", req.Filename)
	if _, err := os.Stat(imagePath); os.IsNotExist(err) {
		jsonError(w, "Image not found", http.StatusBadRequest)
		return
	}

	// Read image, decode, and re-encode as JPEG for moondream2 compatibility
	imgFile, err := os.Open(imagePath)
	if err != nil {
		jsonError(w, "Failed to read image", http.StatusInternalServerError)
		return
	}
	defer imgFile.Close()

	img, _, err := image.Decode(imgFile)
	if err != nil {
		jsonError(w, fmt.Sprintf("Failed to decode image: %v", err), http.StatusInternalServerError)
		return
	}

	// Re-encode as JPEG (moondream2 llama.cpp doesn't support webp)
	var jpegBuf bytes.Buffer
	if err := jpeg.Encode(&jpegBuf, img, &jpeg.Options{Quality: 80}); err != nil {
		jsonError(w, "Failed to convert image", http.StatusInternalServerError)
		return
	}

	imageBase64 := fmt.Sprintf("data:image/jpeg;base64,%s", base64.StdEncoding.EncodeToString(jpegBuf.Bytes()))
	fmt.Printf("AutoPrompt: Image converted to JPEG (%d KB)\n", jpegBuf.Len()/1024)

	// Build style-specific instruction
	styleDesc := map[string]string{
		"cinematic": "cinematic commercial with slow camera orbit, dramatic studio lighting, shallow depth of field",
		"rotating":  "360-degree product rotation on a turntable, even lighting, smooth spin",
		"lifestyle": "lifestyle scene in a real-world environment, warm natural lighting, a hand interacting with the product",
		"tiktok":    "fast-paced TikTok ad with dynamic zoom, product sliding or dropping into frame, high contrast",
		"unboxing":  "POV unboxing video, hands opening a premium box to reveal the product, slow motion",
		"minimal":   "minimal clean shot, product on a smooth surface, soft shadows, subtle camera drift",
	}

	hint := styleDesc[req.Style]
	if hint == "" {
		hint = "cinematic product commercial"
	}

	dur := req.Duration
	if dur < 1 {
		dur = 5
	}

	productCtx := "a product"
	if req.ProductName != "" {
		productCtx = req.ProductName
	}

	userPrompt := fmt.Sprintf(
		"You are writing a prompt for an AI video generator to create an advertisement for %s (shown in the image). "+
			"Look at the image to understand the product's shape, color, and features. "+
			"Write a video ad prompt that includes: the scene or environment, camera movement, lighting, mood, and any action or motion that would sell this product. "+
			"The video is %d seconds long. Style: %s. "+
			"Do NOT describe the product's appearance in detail — the image is already provided to the video generator. "+
			"Focus on how the product is showcased: the setting, motion, interactions, and cinematic direction. "+
			"No emojis, no hashtags, no social media language. "+
			"Output only the prompt, nothing else.",
		productCtx, dur, hint,
	)

	// Build OpenAI-compatible vision request for Docker Model Runner
	chatPayload := map[string]interface{}{
		"model": modelRunnerModel,
		"messages": []map[string]interface{}{
			{
				"role": "user",
				"content": []map[string]interface{}{
					{
						"type": "text",
						"text": userPrompt,
					},
					{
						"type": "image_url",
						"image_url": map[string]string{
							"url": imageBase64,
						},
					},
				},
			},
		},
		"max_tokens": 300,
	}

	chatBody, _ := json.Marshal(chatPayload)

	fmt.Printf("AutoPrompt: Sending image to %s (%s)...\n", modelRunnerModel, req.Style)

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
		Filenames    []string `json:"filenames"`
		Style        string   `json:"style"`
		Ratio        string   `json:"ratio"`
		CustomPrompt string   `json:"custom_prompt"`
		Count        int      `json:"count"`
		Duration     int      `json:"duration"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if len(req.Filenames) == 0 {
		jsonError(w, "filenames is required", http.StatusBadRequest)
		return
	}

	var imagePaths []string
	for _, fn := range req.Filenames {
		p := filepath.Join("uploads", fn)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			jsonError(w, fmt.Sprintf("Image not found: %s", fn), http.StatusBadRequest)
			return
		}
		imagePaths = append(imagePaths, p)
	}

	// Get style config (auto-selects best model)
	cfg, ok := styleConfigs[req.Style]
	if !ok {
		cfg = styleConfigs["cinematic"]
	}

	finalPrompt := cfg.Prompt
	if req.CustomPrompt != "" {
		finalPrompt = cfg.Prompt + " " + req.CustomPrompt
	}

	// Default ratio
	ratio := req.Ratio
	if _, ok := ratioSizes[ratio]; !ok {
		ratio = "9:16"
	}

	// Generate 4 by default
	count := req.Count
	if count < 1 || count > 4 {
		count = 4
	}

	// Duration depends on model
	dur := req.Duration
	if dur < 1 || dur > 16 {
		dur = 5
	}

	jobIDs := make([]string, count)
	for i := 0; i < count; i++ {
		job := &Job{
			ID:         uuid.New().String()[:12],
			Status:     "processing",
			ImagePaths: imagePaths,
			Prompt:     finalPrompt,
			Style:     req.Style,
			Ratio:     ratio,
			Duration:  dur,
			Model:     cfg.ModelName,
			CreatedAt: time.Now().Format(time.RFC3339),
		}

		jobsMu.Lock()
		jobs[job.ID] = job
		jobsMu.Unlock()

		jobIDs[i] = job.ID

		if useMock {
			go mockGenerate(job)
		} else {
			go runwareGenerate(job)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"job_ids": jobIDs,
		"count":   count,
		"status":  "processing",
		"message": fmt.Sprintf("%d video generations started", count),
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
	fmt.Printf("Job %s: Model=%s Style=%s Images=%d\n", job.ID, job.Model, job.Style, len(job.ImagePaths))
	fmt.Printf("Job %s: Prompt=%s\n", job.ID, job.Prompt)

	// Build frameImages from all uploaded images
	var frameImages []map[string]interface{}
	for i, imgPath := range job.ImagePaths {
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

		// Set frame positions: first image = "first", last image = "last", middle = auto
		if len(job.ImagePaths) == 1 {
			frame["frame"] = "first"
		} else if i == 0 {
			frame["frame"] = "first"
		} else if i == len(job.ImagePaths)-1 {
			frame["frame"] = "last"
		}
		// middle images: omit frame param → Runware auto-distributes

		frameImages = append(frameImages, frame)
	}

	// Resolve model ID from style config
	cfg, ok := styleConfigs[job.Style]
	if !ok {
		cfg = styleConfigs["cinematic"]
	}
	runwareModel := cfg.Model

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
		"model":          runwareModel,
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
	case runwareModel == "google:3@3" || runwareModel == "google:3@2" || runwareModel == "google:3@1" || runwareModel == "google:3@0":
		payload["fps"] = 24
		payload["providerSettings"] = map[string]interface{}{
			"google": map[string]interface{}{
				"generateAudio": true,
				"enhancePrompt": true,
			},
		}
	case runwareModel == "vidu:4@2" || runwareModel == "vidu:4@1":
		payload["providerSettings"] = map[string]interface{}{
			"vidu": map[string]interface{}{
				"audio": true,
			},
		}
	case runwareModel == "pixverse:1@7":
		payload["providerSettings"] = map[string]interface{}{
			"pixverse": map[string]interface{}{
				"thinking": "auto",
			},
		}
	}

	reqPayload := []map[string]interface{}{payload}

	reqBody, _ := json.Marshal(reqPayload)

	fmt.Printf("Job %s: Calling Runware (%s)...\n", job.ID, runwareModel)

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

	// Parse response — Runware wraps in {"data": [...]}
	var response struct {
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		setJobError(job, fmt.Sprintf("Failed to parse response: %v", err))
		return
	}

	// Check for direct video URL (some models return immediately)
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

		// Check for API errors — fail immediately, don't keep polling
		for _, e := range pollResp.Errors {
			if msg, ok := e["message"].(string); ok && msg != "" {
				setJobError(job, msg)
				return
			}
		}

		for _, result := range pollResp.Data {
			status, _ := result["status"].(string)

			// Success — get the video URL
			if status == "success" {
				if videoURL, ok := result["videoURL"].(string); ok && videoURL != "" {
					completeJobWithVideo(job, videoURL)
					return
				}
			}

			// Error — fail immediately
			if status == "error" {
				errMsg := "Unknown error"
				if msg, ok := result["message"].(string); ok {
					errMsg = msg
				}
				setJobError(job, errMsg)
				return
			}

			// status == "processing" → keep polling
		}
	}

	setJobError(job, "Timed out waiting for video")
}

func completeJobWithVideo(job *Job, remoteURL string) {
	fmt.Printf("Job %s: Done! Downloading %s\n", job.ID, remoteURL)

	// Download video to local videos/ folder
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
	json.NewEncoder(w).Encode(job)
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

func handleListModels(w http.ResponseWriter, r *http.Request) {
	type StyleInfo struct {
		Style     string  `json:"style"`
		ModelName string  `json:"model_name"`
		Price     float64 `json:"price"`
	}

	var styles []StyleInfo
	for style, cfg := range styleConfigs {
		styles = append(styles, StyleInfo{
			Style:     style,
			ModelName: cfg.ModelName,
			Price:     cfg.Price,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(styles)
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
