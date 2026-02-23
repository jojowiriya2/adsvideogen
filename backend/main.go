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
		return // .env is optional
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
		Prompt:    "POV first-person unboxing video. Start with a closed sleek premium cardboard packaging box on a clean table. Two hands slowly lift the lid off the separate cardboard box. Inside the box, the product is gradually revealed. The box is NOT the product — it is separate outer packaging that contains the product. Smooth slow motion, soft natural lighting, ASMR satisfying reveal moment. The final frame shows the product fully revealed out of the box.",
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
	mux.HandleFunc("POST /api/upload-frame", handleUploadFrame)
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

func handleUploadFrame(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ImageData string `json:"image_data"` // base64 data URL from canvas
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.ImageData == "" {
		jsonError(w, "image_data is required", http.StatusBadRequest)
		return
	}

	// Strip data URL prefix: "data:image/jpeg;base64,..." → raw base64
	b64Data := req.ImageData
	if idx := bytes.IndexByte([]byte(b64Data), ','); idx >= 0 {
		b64Data = b64Data[idx+1:]
	}

	decoded, err := base64.StdEncoding.DecodeString(b64Data)
	if err != nil {
		jsonError(w, "Invalid base64 image data", http.StatusBadRequest)
		return
	}

	filename := "frame-" + uuid.New().String()[:8] + ".jpg"
	savePath := filepath.Join("uploads", filename)

	if err := os.WriteFile(savePath, decoded, 0644); err != nil {
		jsonError(w, "Failed to save frame", http.StatusInternalServerError)
		return
	}

	fmt.Printf("UploadFrame: Saved %s (%d KB)\n", filename, len(decoded)/1024)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"filename":  filename,
		"image_url": fmt.Sprintf("http://localhost:8080/uploads/%s", filename),
	})
}

func handleAutoPrompt(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Filenames      []string `json:"filenames"`
		Style          string   `json:"style"`
		Duration       int      `json:"duration"`
		ProductName    string   `json:"product_name"`
		IsContinuation bool     `json:"is_continuation"`
		PreviousPrompt string   `json:"previous_prompt"`
		FrameFilename  string   `json:"frame_filename"`
		SegmentNumber  int      `json:"segment_number"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Build list of image filenames to send to LLM
	var imageFilenames []string
	if req.IsContinuation && req.FrameFilename != "" {
		// Continuation: send the captured last frame
		imageFilenames = append(imageFilenames, req.FrameFilename)
	}
	// Always include all uploaded product images so LLM sees every angle
	imageFilenames = append(imageFilenames, req.Filenames...)

	if len(imageFilenames) == 0 {
		jsonError(w, "filenames is required", http.StatusBadRequest)
		return
	}

	// Encode all images as JPEG base64
	var imageBase64s []string
	for _, fn := range imageFilenames {
		imgPath := filepath.Join("uploads", fn)
		if _, err := os.Stat(imgPath); os.IsNotExist(err) {
			continue // skip missing files
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

	// Build style-specific instruction
	styleDesc := map[string]string{
		"cinematic": "cinematic commercial with slow camera orbit, dramatic studio lighting, shallow depth of field",
		"rotating":  "360-degree product rotation on a turntable, even lighting, smooth spin",
		"lifestyle": "lifestyle scene in a real-world environment, warm natural lighting, a hand interacting with the product",
		"tiktok":    "fast-paced TikTok ad with dynamic zoom, product sliding or dropping into frame, high contrast",
		"unboxing":  "POV unboxing video, hands opening a separate cardboard packaging box (NOT the product itself) to reveal the product inside at the end, slow motion, ASMR aesthetic",
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

	var userPrompt string
	if req.IsContinuation {
		// Continuation prompt: LLM sees the last frame and must write a follow-up segment
		continueEnding := ""
		if dur >= 6 {
			continueEnding = "The video should end by transitioning back to a clean shot of the product, since the original product image will be used as the last frame. "
		}
		userPrompt = fmt.Sprintf(
			"You are writing a continuation prompt for an AI video generator. This is segment #%d of a multi-part advertisement for %s. "+
				"The attached image is the LAST FRAME of the previous video segment. "+
				"The previous segment's prompt was: \"%s\" "+
				"Write a NEW video prompt that continues seamlessly from this frame. The transition should feel natural and connected. "+
				"The video is %d seconds long. Style: %s. "+
				"%s"+
				"Do NOT repeat what the previous segment already showed. Introduce a new angle, scene, or movement that advances the ad story. "+
				"No emojis, no hashtags, no social media language. "+
				"Output only the prompt, nothing else.",
			req.SegmentNumber, productCtx, req.PreviousPrompt, dur, hint, continueEnding,
		)
	} else {
		imageCtx := ""
		if len(imageBase64s) > 1 {
			imageCtx = fmt.Sprintf("You are given %d images of the product from different angles. The first image is the start frame and the last image is the end frame of the video. ", len(imageBase64s))
		}
		userPrompt = fmt.Sprintf(
			"You are writing a prompt for an AI video generator to create an advertisement for %s (shown in the images). "+
				"%s"+
				"Look at all the images to understand the product's shape, color, and features from every angle. "+
				"Write a video ad prompt that includes: the scene or environment, camera movement, lighting, mood, and any action or motion that would sell this product. "+
				"The video is %d seconds long. Style: %s. "+
				"Do NOT describe the product's appearance in detail — the images are already provided to the video generator. "+
				"Focus on how the product is showcased: the setting, motion, interactions, and cinematic direction. "+
				"If multiple angles are provided, incorporate the transition between them (e.g. closed to open, front to back). "+
				"No emojis, no hashtags, no social media language. "+
				"Output only the prompt, nothing else.",
			productCtx, imageCtx, dur, hint,
		)
	}

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

	fmt.Printf("AutoPrompt: Sending %d image(s) to %s (%s)...\n", len(imageBase64s), modelRunnerModel, req.Style)

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
		Filenames         []string `json:"filenames"`
		Style             string   `json:"style"`
		Ratio             string   `json:"ratio"`
		CustomPrompt      string   `json:"custom_prompt"`
		Count             int      `json:"count"`
		Duration          int      `json:"duration"`
		IsContinuation    bool     `json:"is_continuation"`
		LastFrameFilename string   `json:"last_frame_filename"` // captured last frame
		OriginalFilenames []string `json:"original_filenames"`  // original product images
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if !req.IsContinuation && len(req.Filenames) == 0 {
		jsonError(w, "filenames is required", http.StatusBadRequest)
		return
	}
	if req.IsContinuation && req.LastFrameFilename == "" {
		jsonError(w, "last_frame_filename is required for continuation", http.StatusBadRequest)
		return
	}

	var imagePaths []string
	if req.IsContinuation {
		// Continuation: last frame = first, original product image = last (if duration >= 6s)
		lastFramePath := filepath.Join("uploads", req.LastFrameFilename)
		if _, err := os.Stat(lastFramePath); os.IsNotExist(err) {
			jsonError(w, "Last frame image not found", http.StatusBadRequest)
			return
		}
		imagePaths = append(imagePaths, lastFramePath)

		if req.Duration >= 6 && len(req.OriginalFilenames) > 0 {
			// Use first original product image as last frame anchor
			origPath := filepath.Join("uploads", req.OriginalFilenames[0])
			if _, err := os.Stat(origPath); os.IsNotExist(err) {
				jsonError(w, "Original product image not found", http.StatusBadRequest)
				return
			}
			imagePaths = append(imagePaths, origPath)
		}
	} else {
		for _, fn := range req.Filenames {
			p := filepath.Join("uploads", fn)
			if _, err := os.Stat(p); os.IsNotExist(err) {
				jsonError(w, fmt.Sprintf("Image not found: %s", fn), http.StatusBadRequest)
				return
			}
			imagePaths = append(imagePaths, p)
		}
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

	// Clamp to max 2 images (first + last) — all current models only support 1-2 frameImages
	usePaths := job.ImagePaths
	if len(usePaths) > 2 {
		usePaths = []string{usePaths[0], usePaths[len(usePaths)-1]}
		fmt.Printf("Job %s: Clamped %d images → 2 (first + last)\n", job.ID, len(job.ImagePaths))
	}

	// Build frameImages from uploaded images
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

		// Set frame positions based on style
		if job.Style == "unboxing" {
			// Unboxing: product is the REVEAL at the end, not the start
			if len(usePaths) == 1 {
				frame["frame"] = "last"
			} else if i == 0 {
				frame["frame"] = "first"
			} else if i == len(usePaths)-1 {
				frame["frame"] = "last"
			}
		} else {
			// All other styles: first image = start, last image = end
			if len(usePaths) == 1 {
				frame["frame"] = "first"
			} else if i == 0 {
				frame["frame"] = "first"
			} else if i == len(usePaths)-1 {
				frame["frame"] = "last"
			}
		}

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
