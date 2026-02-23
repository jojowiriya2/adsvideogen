# Product Video AI

Upload product images, generate AI video ad variations, pick the best one.

Next.js frontend + Go backend + [Runware.ai](https://runware.ai) API for video generation + Docker Model Runner for auto-prompt.

## Quick Start

```bash
# 1. Clone
git clone https://github.com/jojowiriya2/adsvideogen.git
cd adsvideogen

# 2. Set up env
cp backend/.env.example backend/.env
# Edit backend/.env and add your RUNWARE_API_KEY

# 3. Install frontend
cd frontend && npm install && cd ..

# 4. Start backend (terminal 1)
cd backend && go run main.go

# 5. Start frontend (terminal 2)
cd frontend && npm run dev

# 6. Open http://localhost:3000
```

## Prerequisites

- [Go](https://go.dev/dl/) 1.21+
- [Node.js](https://nodejs.org/) 18+
- [Docker Desktop](https://www.docker.com/products/docker-desktop/) 4.40+ (for Model Runner)
- A [Runware.ai](https://runware.ai) API key

## Setup

### 1. Clone the repo

```bash
git clone https://github.com/jojowiriya2/adsvideogen.git
cd adsvideogen
```

### 2. Set up Docker Model Runner (for Auto-Prompt)

Docker Model Runner runs a local LLM that analyzes your product images and writes video prompts automatically. It uses Gemma 3 4B — a small vision model that runs on your machine.

**Step 1: Enable Docker Model Runner**

Open Docker Desktop → Settings → Features in development → Enable "Docker Model Runner". Click "Apply & restart".

Alternatively, enable via CLI:

```bash
docker desktop enable model-runner
```

**Step 2: Pull the Gemma 3 model**

```bash
docker model pull ai/gemma3:4B-Q4_K_M
```

This downloads ~3GB. Wait for it to finish.

**Step 3: Verify it's running**

```bash
curl http://localhost:12434/engines/llama.cpp/v1/models
```

You should see a JSON response listing the model. The Model Runner URL is:

```
http://localhost:12434/engines/llama.cpp/v1/chat/completions
```

This is the default `MODEL_RUNNER_URL` in the `.env` file.

> **Note:** Docker Model Runner is only needed for the "Auto Prompt" feature. You can skip it and write prompts manually.

### 3. Get a Runware API key

1. Sign up at [runware.ai](https://runware.ai)
2. Go to your dashboard and copy your API key

### 4. Configure environment variables

```bash
cp backend/.env.example backend/.env
```

Edit `backend/.env` and fill in your values:

```env
RUNWARE_API_KEY=your_runware_api_key_here
MODEL_RUNNER_URL=http://localhost:12434/engines/llama.cpp/v1/chat/completions
MODEL_RUNNER_MODEL=ai/gemma3:4B-Q4_K_M
```

| Variable | Description |
|---|---|
| `RUNWARE_API_KEY` | Your Runware.ai API key (required) |
| `MODEL_RUNNER_URL` | Docker Model Runner endpoint (default works if Docker Model Runner is enabled) |
| `MODEL_RUNNER_MODEL` | Vision LLM model ID (default: Gemma 3 4B) |

### 5. Install frontend dependencies

```bash
cd frontend
npm install
cd ..
```

## Running

Open two terminals:

**Terminal 1 — Backend (Go)**

```bash
cd backend
go run main.go
```

Runs on http://localhost:8080

**Terminal 2 — Frontend (Next.js)**

```bash
cd frontend
npm run dev
```

Runs on http://localhost:3000

Open http://localhost:3000 in your browser.

## How It Works

1. Upload 1-2 product images
2. Pick an ad style (Cinematic, 360 Rotating, Lifestyle, TikTok, POV Unboxing, Minimal)
3. Click "Auto Prompt" to let the LLM write a video prompt based on your images, or write your own
4. Hit Generate — Runware.ai creates the video
5. Download or continue chaining segments for longer ads

## Ad Styles & Models

| Style | Model | Price/video |
|---|---|---|
| Cinematic | Veo 3.1 Fast | $0.80 |
| 360 Rotating | Vidu Q3 Turbo | $0.13 |
| Lifestyle | PixVerse v5.6 | $0.24 |
| TikTok / Reels | Vidu Q3 | $0.05 |
| POV Unboxing | Vidu Q3 Turbo | $0.13 |
| Minimal Clean | Vidu Q3 | $0.05 |

## Project Structure

```
product-video-app/
├── backend/
│   ├── main.go          # Go API server
│   ├── .env             # API keys (git-ignored)
│   ├── .env.example     # Template
│   ├── uploads/         # Uploaded images
│   └── videos/          # Downloaded generated videos
├── frontend/
│   └── src/app/page.tsx # Main UI
└── README.md
```
