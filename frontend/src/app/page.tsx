"use client";

import { useState, useCallback, useRef, useEffect } from "react";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

const API_URL = "http://localhost:8080";

const AD_STYLES = [
  {
    value: "cinematic",
    label: "Cinematic",
    model: "Veo 3.1 Fast",
    priceNum: 0.80,
    desc: "Google Veo 3.1 — best quality with native audio",
    durations: [4, 6, 8],
    estSeconds: 120,
  },
  {
    value: "rotating",
    label: "360 Rotating",
    model: "Vidu Q3 Turbo",
    priceNum: 0.13,
    desc: "Product spin on white background",
    durations: [5, 8, 16],
    estSeconds: 60,
  },
  {
    value: "lifestyle",
    label: "Lifestyle",
    model: "PixVerse v5.6",
    priceNum: 0.24,
    desc: "Real-world setting, warm social media aesthetic",
    durations: [5, 8],
    estSeconds: 90,
  },
  {
    value: "tiktok",
    label: "TikTok / Reels",
    model: "Vidu Q3",
    priceNum: 0.05,
    desc: "Fast-paced, bold colors, dynamic transitions",
    durations: [5, 8, 16],
    estSeconds: 60,
  },
  {
    value: "unboxing",
    label: "POV Unboxing",
    model: "Vidu Q3 Turbo",
    priceNum: 0.13,
    desc: "Satisfying unboxing reveal, POV style",
    durations: [5, 8, 16],
    estSeconds: 60,
  },
  {
    value: "minimal",
    label: "Minimal Clean",
    model: "Vidu Q3",
    priceNum: 0.05,
    desc: "Simple surface, soft shadows, modern look",
    durations: [5, 8, 16],
    estSeconds: 60,
  },
];

const RATIOS = [
  { value: "9:16", label: "Mobile", desc: "9:16 · TikTok, Reels, Stories" },
  { value: "16:9", label: "Desktop", desc: "16:9 · YouTube, Website" },
  { value: "1:1", label: "Square", desc: "1:1 · Instagram Feed" },
];

type VideoResult = {
  id: string;
  status: "processing" | "completed" | "failed";
  video_url?: string;
  error?: string;
};

type ChainSegment = {
  videoUrl: string;
  prompt: string;
  style: string;
  duration: number;
  jobId: string;
};

type UploadedImage = {
  file: File;
  previewUrl: string;
  filename: string | null;
};

const MAX_IMAGES = 4;

export default function Home() {
  const [images, setImages] = useState<UploadedImage[]>([]);
  const [productName, setProductName] = useState("");
  const [style, setStyle] = useState("tiktok");
  const [ratio, setRatio] = useState("9:16");
  const [videoCount, setVideoCount] = useState(1);
  const [duration, setDuration] = useState(5);
  const [customPrompt, setCustomPrompt] = useState("");
  const [isUploading, setIsUploading] = useState(false);
  const [isGenerating, setIsGenerating] = useState(false);
  const [videos, setVideos] = useState<VideoResult[]>([]);
  const [selectedVideo, setSelectedVideo] = useState<string | null>(null);
  const [dragActive, setDragActive] = useState(false);
  const [isAutoPrompting, setIsAutoPrompting] = useState(false);
  const [elapsed, setElapsed] = useState(0);
  const pollingRef = useRef(false);
  const startTimeRef = useRef<number>(0);

  // Chain state
  const [chain, setChain] = useState<ChainSegment[]>([]);
  const [isContinuing, setIsContinuing] = useState(false);
  const [continuationPrompt, setContinuationPrompt] = useState("");
  const [showContinuePanel, setShowContinuePanel] = useState(false);
  const [continuationFrameFilename, setContinuationFrameFilename] = useState<string | null>(null);

  const allUploaded = images.length > 0 && images.every((img) => img.filename !== null);
  const uploadedFilenames = images.filter((img) => img.filename).map((img) => img.filename!);

  const totalChainDuration = chain.reduce((sum, seg) => sum + seg.duration, 0);

  // Elapsed timer during generation
  useEffect(() => {
    if (!isGenerating) {
      setElapsed(0);
      return;
    }
    startTimeRef.current = Date.now();
    const interval = setInterval(() => {
      setElapsed(Math.floor((Date.now() - startTimeRef.current) / 1000));
    }, 1000);
    return () => clearInterval(interval);
  }, [isGenerating]);

  const addFiles = (files: FileList | File[]) => {
    const newImages: UploadedImage[] = [];
    for (const file of Array.from(files)) {
      if (!file.type.startsWith("image/")) continue;
      if (images.length + newImages.length >= MAX_IMAGES) break;
      newImages.push({ file, previewUrl: URL.createObjectURL(file), filename: null });
    }
    if (newImages.length > 0) {
      setImages((prev) => [...prev, ...newImages]);
      setVideos([]);
      setSelectedVideo(null);
      setChain([]);
      setShowContinuePanel(false);
    }
  };

  const removeImage = (index: number) => {
    setImages((prev) => {
      const copy = [...prev];
      URL.revokeObjectURL(copy[index].previewUrl);
      copy.splice(index, 1);
      return copy;
    });
    setVideos([]);
    setSelectedVideo(null);
    setChain([]);
    setShowContinuePanel(false);
  };

  const onDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setDragActive(false);
    if (e.dataTransfer.files?.length) {
      addFiles(e.dataTransfer.files);
    }
  }, [images.length]);

  const onDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setDragActive(true);
  }, []);

  const onDragLeave = useCallback(() => {
    setDragActive(false);
  }, []);

  const uploadImages = async () => {
    const pending = images.filter((img) => img.filename === null);
    if (pending.length === 0) return;
    setIsUploading(true);

    try {
      for (let i = 0; i < images.length; i++) {
        if (images[i].filename !== null) continue;
        const formData = new FormData();
        formData.append("image", images[i].file);
        const res = await fetch(`${API_URL}/api/upload`, {
          method: "POST",
          body: formData,
        });
        const data = await res.json();
        if (data.error) throw new Error(data.error);
        setImages((prev) => {
          const copy = [...prev];
          copy[i] = { ...copy[i], filename: data.filename };
          return copy;
        });
      }
    } catch (err) {
      alert("Upload failed: " + (err as Error).message);
    } finally {
      setIsUploading(false);
    }
  };

  // Capture last frame from a video element via canvas
  const captureLastFrame = (videoUrl: string): Promise<string> => {
    return new Promise((resolve, reject) => {
      const video = document.createElement("video");
      video.crossOrigin = "anonymous";
      video.src = videoUrl;
      video.muted = true;

      video.onloadedmetadata = () => {
        // Seek to the last frame (duration - small offset)
        video.currentTime = Math.max(0, video.duration - 0.05);
      };

      video.onseeked = () => {
        const canvas = document.createElement("canvas");
        canvas.width = video.videoWidth;
        canvas.height = video.videoHeight;
        const ctx = canvas.getContext("2d");
        if (!ctx) {
          reject(new Error("Canvas context failed"));
          return;
        }
        ctx.drawImage(video, 0, 0);
        const dataUrl = canvas.toDataURL("image/jpeg", 0.9);
        resolve(dataUrl);
      };

      video.onerror = () => reject(new Error("Failed to load video"));
    });
  };

  // Upload a captured frame (base64) to the server
  const uploadFrame = async (dataUrl: string): Promise<string> => {
    const res = await fetch(`${API_URL}/api/upload-frame`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ image_data: dataUrl }),
    });
    const data = await res.json();
    if (data.error) throw new Error(data.error);
    return data.filename;
  };

  const generateVideos = async () => {
    if (!allUploaded) return;

    setIsGenerating(true);
    setVideos([]);
    setSelectedVideo(null);
    setShowContinuePanel(false);
    pollingRef.current = true;

    try {
      const res = await fetch(`${API_URL}/api/generate`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          filenames: uploadedFilenames,
          style: style,
          ratio: ratio,
          custom_prompt: customPrompt,
          count: videoCount,
          duration: duration,
        }),
      });
      const data = await res.json();
      if (data.error) throw new Error(data.error);

      const jobIds: string[] = data.job_ids;
      setVideos(jobIds.map((id) => ({ id, status: "processing" as const })));

      const completed = new Set<string>();
      while (completed.size < jobIds.length && pollingRef.current) {
        await new Promise((r) => setTimeout(r, 3000));
        for (const jobId of jobIds) {
          if (completed.has(jobId)) continue;
          try {
            const statusRes = await fetch(`${API_URL}/api/status/${jobId}`);
            const status = await statusRes.json();
            if (status.status === "completed" || status.status === "failed") {
              completed.add(jobId);
              setVideos((prev) =>
                prev.map((v) =>
                  v.id === jobId
                    ? { ...v, status: status.status, video_url: status.video_url, error: status.error }
                    : v
                )
              );
            }
          } catch {
            // ignore polling errors
          }
        }
      }
    } catch (err) {
      alert("Generation failed: " + (err as Error).message);
    } finally {
      setIsGenerating(false);
      pollingRef.current = false;
    }
  };

  // Generate continuation segment
  const generateContinuation = async () => {
    if (!continuationFrameFilename || !continuationPrompt.trim()) return;

    setIsGenerating(true);
    setVideos([]);
    setShowContinuePanel(false);
    pollingRef.current = true;

    try {
      const res = await fetch(`${API_URL}/api/generate`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          style: style,
          ratio: ratio,
          custom_prompt: continuationPrompt,
          count: 1,
          duration: duration,
          is_continuation: true,
          last_frame_filename: continuationFrameFilename,
          original_filenames: uploadedFilenames,
        }),
      });
      const data = await res.json();
      if (data.error) throw new Error(data.error);

      const jobIds: string[] = data.job_ids;
      setVideos(jobIds.map((id) => ({ id, status: "processing" as const })));

      const completed = new Set<string>();
      while (completed.size < jobIds.length && pollingRef.current) {
        await new Promise((r) => setTimeout(r, 3000));
        for (const jobId of jobIds) {
          if (completed.has(jobId)) continue;
          try {
            const statusRes = await fetch(`${API_URL}/api/status/${jobId}`);
            const status = await statusRes.json();
            if (status.status === "completed" || status.status === "failed") {
              completed.add(jobId);
              setVideos((prev) =>
                prev.map((v) =>
                  v.id === jobId
                    ? { ...v, status: status.status, video_url: status.video_url, error: status.error }
                    : v
                )
              );
            }
          } catch {
            // ignore
          }
        }
      }
    } catch (err) {
      alert("Continuation failed: " + (err as Error).message);
    } finally {
      setIsGenerating(false);
      pollingRef.current = false;
    }
  };

  const autoPrompt = async () => {
    if (uploadedFilenames.length === 0) return;
    setIsAutoPrompting(true);
    try {
      const res = await fetch(`${API_URL}/api/auto-prompt`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ filename: uploadedFilenames[0], style, duration, product_name: productName }),
      });
      const data = await res.json();
      if (data.error) throw new Error(data.error);
      setCustomPrompt(data.prompt);
    } catch (err) {
      alert("Auto prompt failed: " + (err as Error).message);
    } finally {
      setIsAutoPrompting(false);
    }
  };

  // Auto-prompt for continuation (sends last frame to LLM)
  const autoContinuationPrompt = async () => {
    if (!continuationFrameFilename) return;
    setIsAutoPrompting(true);
    try {
      const lastSegment = chain[chain.length - 1];
      const res = await fetch(`${API_URL}/api/auto-prompt`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          filename: uploadedFilenames[0],
          style,
          duration,
          product_name: productName,
          is_continuation: true,
          previous_prompt: lastSegment?.prompt || customPrompt,
          frame_filename: continuationFrameFilename,
          segment_number: chain.length + 1,
        }),
      });
      const data = await res.json();
      if (data.error) throw new Error(data.error);
      setContinuationPrompt(data.prompt);
    } catch (err) {
      alert("Auto prompt failed: " + (err as Error).message);
    } finally {
      setIsAutoPrompting(false);
    }
  };

  // Start continuation flow: capture last frame → upload → show panel
  const startContinuation = async (videoUrl: string, videoJobId: string) => {
    setIsContinuing(true);
    try {
      // 1. Capture last frame via canvas
      const frameDataUrl = await captureLastFrame(videoUrl);

      // 2. Upload frame to server
      const frameFilename = await uploadFrame(frameDataUrl);
      setContinuationFrameFilename(frameFilename);

      // 3. Add current video to chain if not already there
      const lastPrompt = chain.length > 0 ? chain[chain.length - 1].prompt : customPrompt;
      const alreadyInChain = chain.some((s) => s.jobId === videoJobId);
      if (!alreadyInChain) {
        setChain((prev) => [
          ...prev,
          {
            videoUrl,
            prompt: lastPrompt,
            style,
            duration,
            jobId: videoJobId,
          },
        ]);
      }

      // 4. Show continuation panel
      setContinuationPrompt("");
      setShowContinuePanel(true);
    } catch (err) {
      alert("Failed to capture frame: " + (err as Error).message);
    } finally {
      setIsContinuing(false);
    }
  };

  // Add completed continuation video to chain
  const addToChain = (video: VideoResult) => {
    if (!video.video_url) return;
    setChain((prev) => [
      ...prev,
      {
        videoUrl: video.video_url!,
        prompt: continuationPrompt,
        style,
        duration,
        jobId: video.id,
      },
    ]);
    setContinuationPrompt("");
    setContinuationFrameFilename(null);
    setShowContinuePanel(false);
  };

  const reset = () => {
    pollingRef.current = false;
    images.forEach((img) => URL.revokeObjectURL(img.previewUrl));
    setImages([]);
    setVideos([]);
    setSelectedVideo(null);
    setChain([]);
    setShowContinuePanel(false);
    setContinuationPrompt("");
    setContinuationFrameFilename(null);
  };

  const selectedStyle = AD_STYLES.find((s) => s.value === style)!;
  const completedVideos = videos.filter((v) => v.status === "completed");
  const processingCount = videos.filter((v) => v.status === "processing").length;

  const ratioStyle = {
    aspectRatio: ratio.replace(":", " / "),
  };

  return (
    <div className="min-h-screen bg-zinc-950 text-white">
      <header className="border-b border-zinc-800 px-6 py-4">
        <div className="mx-auto flex max-w-6xl items-center justify-between">
          <h1 className="text-xl font-bold">Product Video AI</h1>
          <div className="flex items-center gap-3">
            {chain.length > 0 && (
              <Badge variant="secondary" className="bg-purple-600/20 text-purple-300">
                {chain.length} segment{chain.length > 1 ? "s" : ""} · {totalChainDuration}s total
              </Badge>
            )}
            <Badge variant="secondary">Demo</Badge>
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-6xl px-6 py-10">
        <div className="mb-8 text-center">
          <h2 className="text-3xl font-bold">
            Turn Product Images Into Ad Videos
          </h2>
          <p className="mt-2 text-zinc-400">
            Upload a product photo — AI generates video variations for you to
            pick from
          </p>
        </div>

        {/* Top: Upload + Settings */}
        <div className="mb-8 grid gap-6 md:grid-cols-3">
          {/* Upload */}
          <Card className="border-zinc-800 bg-zinc-900 p-5">
            <h3 className="mb-1 font-semibold text-white">
              1. Product Images
            </h3>
            <p className="mb-3 text-[10px] text-zinc-500">
              Up to {MAX_IMAGES} angles — first image = start frame, last = end frame
            </p>

            <div className="grid grid-cols-2 gap-2">
              {images.map((img, i) => (
                <div key={i} className="relative group">
                  <img
                    src={img.previewUrl}
                    alt={`Angle ${i + 1}`}
                    className="h-20 w-full rounded-lg object-contain bg-zinc-800"
                  />
                  <button
                    onClick={() => removeImage(i)}
                    className="absolute right-1 top-1 rounded-full bg-black/70 p-0.5 text-white opacity-0 transition-opacity group-hover:opacity-100 hover:bg-black"
                  >
                    <svg className="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                    </svg>
                  </button>
                  <span className="absolute bottom-1 left-1 rounded bg-black/70 px-1 py-0.5 text-[9px] text-zinc-300">
                    {i === 0 ? "First" : i === images.length - 1 && images.length > 1 ? "Last" : `#${i + 1}`}
                  </span>
                  {img.filename && (
                    <span className="absolute bottom-1 right-1 rounded bg-green-600/80 px-1 py-0.5 text-[9px] text-white">
                      OK
                    </span>
                  )}
                </div>
              ))}

              {images.length < MAX_IMAGES && (
                <div
                  onDrop={onDrop}
                  onDragOver={onDragOver}
                  onDragLeave={onDragLeave}
                  className={`flex h-20 cursor-pointer flex-col items-center justify-center rounded-lg border-2 border-dashed transition-colors ${
                    dragActive
                      ? "border-blue-500 bg-blue-500/10"
                      : "border-zinc-700 hover:border-zinc-500"
                  }`}
                  onClick={() => document.getElementById("file-input")?.click()}
                >
                  <svg className="mb-1 h-5 w-5 text-zinc-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M12 4v16m8-8H4" />
                  </svg>
                  <p className="text-[10px] text-zinc-500">
                    {images.length === 0 ? "Add image" : "Add angle"}
                  </p>
                  <input
                    id="file-input"
                    type="file"
                    accept="image/jpeg,image/png,image/webp"
                    multiple
                    className="hidden"
                    onChange={(e) => {
                      if (e.target.files?.length) addFiles(e.target.files);
                      e.target.value = "";
                    }}
                  />
                </div>
              )}
            </div>

            {images.length > 0 && !allUploaded && (
              <Button
                className="mt-3 w-full"
                size="sm"
                onClick={uploadImages}
                disabled={isUploading}
              >
                {isUploading
                  ? `Uploading... (${images.filter((i) => i.filename).length}/${images.length})`
                  : `Upload ${images.length} image${images.length > 1 ? "s" : ""}`}
              </Button>
            )}
            {allUploaded && (
              <div className="mt-2 flex items-center justify-between">
                <p className="text-xs text-green-400">
                  {images.length} image{images.length > 1 ? "s" : ""} uploaded
                </p>
                <button onClick={reset} className="text-xs text-zinc-500 hover:text-zinc-300">
                  Clear all
                </button>
              </div>
            )}

            {images.length > 0 && (
              <div className="mt-3">
                <p className="mb-1 text-xs text-zinc-400">Product name</p>
                <input
                  type="text"
                  value={productName}
                  onChange={(e) => setProductName(e.target.value)}
                  placeholder="e.g. wireless earbuds, sunglasses, sneakers..."
                  className="w-full rounded-lg border border-zinc-700 bg-zinc-800 px-2.5 py-1.5 text-xs text-white placeholder-zinc-600 focus:border-blue-500 focus:outline-none"
                />
              </div>
            )}
          </Card>

          {/* Style */}
          <Card className="border-zinc-800 bg-zinc-900 p-5">
            <h3 className="mb-3 font-semibold text-white">2. Ad Style</h3>
            <Select value={style} onValueChange={(v) => {
                setStyle(v);
                const newStyle = AD_STYLES.find((s) => s.value === v)!;
                if (!newStyle.durations.includes(duration)) {
                  setDuration(newStyle.durations[0]);
                }
              }}>
              <SelectTrigger className="border-zinc-700 bg-zinc-800 text-white">
                <SelectValue />
              </SelectTrigger>
              <SelectContent className="border-zinc-700 bg-zinc-800">
                {AD_STYLES.map((s) => (
                  <SelectItem
                    key={s.value}
                    value={s.value}
                    className="text-white hover:bg-zinc-700"
                  >
                    {s.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <div className="mt-3 text-xs text-zinc-500">
              <p>{selectedStyle.desc}</p>
              <p className="mt-1 text-zinc-600">
                Model: {selectedStyle.model} · ${selectedStyle.priceNum.toFixed(2)}/video
              </p>
            </div>

            <h3 className="mb-2 mt-4 font-semibold text-white">Aspect Ratio</h3>
            <div className="grid grid-cols-2 gap-2">
              {RATIOS.map((r) => (
                <button
                  key={r.value}
                  onClick={() => setRatio(r.value)}
                  className={`rounded-lg border px-3 py-2 text-left text-xs transition-colors ${
                    ratio === r.value
                      ? "border-blue-500 bg-blue-500/10 text-white"
                      : "border-zinc-700 bg-zinc-800 text-zinc-400 hover:border-zinc-500"
                  }`}
                >
                  <span className="font-medium">{r.label}</span>
                  <span className="block text-[10px] text-zinc-500">{r.desc}</span>
                </button>
              ))}
            </div>
          </Card>

          {/* Custom Prompt + Generate */}
          <Card className="border-zinc-800 bg-zinc-900 p-5">
            <div className="mb-3 flex items-center justify-between">
              <h3 className="font-semibold text-white">
                3. Extra Details{" "}
                <span className="text-xs font-normal text-zinc-500">
                  (optional)
                </span>
              </h3>
              <button
                onClick={autoPrompt}
                disabled={!allUploaded || isAutoPrompting}
                className="rounded-md bg-purple-600 px-2.5 py-1 text-xs font-medium text-white transition-colors hover:bg-purple-700 disabled:opacity-40 disabled:cursor-not-allowed"
              >
                {isAutoPrompting ? "Analyzing..." : "Auto Prompt"}
              </button>
            </div>
            <textarea
              value={customPrompt}
              onChange={(e) => {
                setCustomPrompt(e.target.value);
                e.target.style.height = "auto";
                e.target.style.height = e.target.scrollHeight + "px";
              }}
              ref={(el) => {
                if (el) {
                  el.style.height = "auto";
                  el.style.height = el.scrollHeight + "px";
                }
              }}
              placeholder="e.g. product glow, water drops... or click Auto Prompt"
              className="w-full resize-none overflow-hidden rounded-lg border border-zinc-700 bg-zinc-800 p-2 text-xs text-white placeholder-zinc-600 focus:border-blue-500 focus:outline-none"
              rows={3}
            />

            <div className="mt-3">
              <p className="mb-2 text-xs text-zinc-400">Duration</p>
              <div className="flex gap-2">
                {selectedStyle.durations.map((d) => (
                  <button
                    key={d}
                    onClick={() => setDuration(d)}
                    className={`flex-1 rounded-lg border py-1.5 text-sm font-medium transition-colors ${
                      duration === d
                        ? "border-blue-500 bg-blue-500/10 text-white"
                        : "border-zinc-700 bg-zinc-800 text-zinc-400 hover:border-zinc-500"
                    }`}
                  >
                    {d}s
                  </button>
                ))}
              </div>
            </div>

            <div className="mt-3">
              <p className="mb-2 text-xs text-zinc-400">Variations</p>
              <div className="flex gap-2">
                {[1, 2, 3, 4].map((n) => (
                  <button
                    key={n}
                    onClick={() => n === 1 && setVideoCount(n)}
                    disabled={n > 1}
                    className={`flex-1 rounded-lg border py-1.5 text-sm font-medium transition-colors ${
                      videoCount === n
                        ? "border-blue-500 bg-blue-500/10 text-white"
                        : n > 1
                          ? "border-zinc-700 bg-zinc-800 text-zinc-600 opacity-40 cursor-not-allowed"
                          : "border-zinc-700 bg-zinc-800 text-zinc-400 hover:border-zinc-500"
                    }`}
                  >
                    {n}
                  </button>
                ))}
              </div>
            </div>

            <Button
              className="mt-3 w-full bg-blue-600 hover:bg-blue-700"
              disabled={!allUploaded || isGenerating}
              onClick={generateVideos}
            >
              {isGenerating
                ? `Generating... (${processingCount} left)`
                : `Generate ${videoCount} Video${videoCount > 1 ? "s" : ""} ($${(selectedStyle.priceNum * videoCount).toFixed(2)})`}
            </Button>
          </Card>
        </div>

        {/* Chain Timeline */}
        {chain.length > 0 && (
          <div className="mb-8">
            <div className="mb-3 flex items-center justify-between">
              <h3 className="text-lg font-semibold">
                Video Chain
                <span className="ml-2 text-sm font-normal text-zinc-500">
                  {totalChainDuration}s total
                </span>
              </h3>
              <button
                onClick={() => { setChain([]); setShowContinuePanel(false); setContinuationFrameFilename(null); }}
                className="text-xs text-zinc-500 hover:text-zinc-300"
              >
                Clear chain
              </button>
            </div>
            <div className="flex gap-2 overflow-x-auto pb-2">
              {chain.map((seg, i) => (
                <div key={seg.jobId} className="flex-shrink-0">
                  <Card className="border-zinc-700 bg-zinc-900 p-2 w-40">
                    <video
                      src={seg.videoUrl}
                      muted
                      playsInline
                      loop
                      autoPlay
                      className="w-full rounded bg-black object-contain"
                      style={{ aspectRatio: ratio.replace(":", " / ") }}
                    />
                    <div className="mt-1.5 flex items-center justify-between">
                      <span className="text-[10px] text-zinc-400">
                        Seg {i + 1} · {seg.duration}s
                      </span>
                      <Badge className="bg-zinc-700 text-[9px] px-1 py-0">
                        {seg.style}
                      </Badge>
                    </div>
                  </Card>
                  {i < chain.length - 1 && (
                    <div className="flex items-center justify-center py-1">
                      <svg className="h-3 w-3 text-zinc-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 14l-7 7m0 0l-7-7m7 7V3" />
                      </svg>
                    </div>
                  )}
                </div>
              ))}
              {/* Next segment placeholder */}
              {showContinuePanel && (
                <div className="flex-shrink-0">
                  <Card className="border-dashed border-purple-500/50 bg-purple-950/20 p-2 w-40 flex items-center justify-center"
                    style={{ minHeight: "120px" }}>
                    <p className="text-[10px] text-purple-400 text-center">Next segment...</p>
                  </Card>
                </div>
              )}
            </div>
          </div>
        )}

        {/* Continuation Panel */}
        {showContinuePanel && (
          <Card className="mb-8 border-purple-500/30 bg-purple-950/10 p-5">
            <h3 className="mb-3 font-semibold text-purple-300">
              Continue Video — Segment #{chain.length + 1}
            </h3>
            <p className="mb-3 text-xs text-zinc-400">
              Last frame captured from segment #{chain.length}. AI will generate the next segment starting from that frame.
              {duration >= 6 && " The original product image will be used as the ending frame."}
              {duration < 6 && " (Short duration — no product ending frame)"}
            </p>

            <div className="mb-3 flex items-center gap-2">
              <button
                onClick={autoContinuationPrompt}
                disabled={isAutoPrompting}
                className="rounded-md bg-purple-600 px-2.5 py-1 text-xs font-medium text-white transition-colors hover:bg-purple-700 disabled:opacity-40 disabled:cursor-not-allowed"
              >
                {isAutoPrompting ? "Analyzing..." : "Auto Prompt (Continuation)"}
              </button>
              <span className="text-[10px] text-zinc-500">
                LLM sees the last frame + knows what came before
              </span>
            </div>

            <textarea
              value={continuationPrompt}
              onChange={(e) => {
                setContinuationPrompt(e.target.value);
                e.target.style.height = "auto";
                e.target.style.height = e.target.scrollHeight + "px";
              }}
              placeholder="Describe what happens next in the ad... or click Auto Prompt"
              className="w-full resize-none overflow-hidden rounded-lg border border-purple-500/30 bg-zinc-900 p-2 text-xs text-white placeholder-zinc-600 focus:border-purple-500 focus:outline-none"
              rows={3}
            />

            <div className="mt-3 flex gap-2">
              <Button
                className="flex-1 bg-purple-600 hover:bg-purple-700"
                disabled={!continuationPrompt.trim() || isGenerating}
                onClick={generateContinuation}
              >
                {isGenerating
                  ? `Generating segment #${chain.length + 1}...`
                  : `Generate Segment #${chain.length + 1} ($${selectedStyle.priceNum.toFixed(2)})`}
              </Button>
              <Button
                variant="outline"
                onClick={() => { setShowContinuePanel(false); setContinuationFrameFilename(null); }}
              >
                Cancel
              </Button>
            </div>
          </Card>
        )}

        {/* Video Results */}
        {videos.length > 0 && (
          <div>
            <h3 className="mb-4 text-lg font-semibold">
              {chain.length > 0 && !showContinuePanel ? "Latest Segment" : "Pick Your Favorite"}{" "}
              {completedVideos.length > 0 && (
                <span className="text-sm font-normal text-zinc-500">
                  — {completedVideos.length}/{videos.length} ready
                </span>
              )}
            </h3>
            <div className={`grid gap-4 ${videos.length > 1 ? "grid-cols-2" : "grid-cols-1 max-w-md mx-auto"}`}>
              {videos.map((video, i) => (
                <Card
                  key={video.id}
                  className={`border-zinc-800 bg-zinc-900 p-3 transition-all cursor-pointer ${
                    selectedVideo === video.id
                      ? "ring-2 ring-blue-500 border-blue-500"
                      : "hover:border-zinc-600"
                  }`}
                  onClick={() =>
                    video.status === "completed" &&
                    setSelectedVideo(video.id)
                  }
                >
                  {video.status === "processing" && (() => {
                    const est = selectedStyle.estSeconds;
                    const pct = Math.min(Math.round((elapsed / est) * 100), 95);
                    const mins = Math.floor(elapsed / 60);
                    const secs = elapsed % 60;
                    const timeStr = mins > 0 ? `${mins}m ${secs}s` : `${secs}s`;
                    return (
                      <div
                        style={ratioStyle}
                        className="flex w-full flex-col items-center justify-center gap-3 rounded-lg border border-zinc-700 px-6"
                      >
                        <div className="h-8 w-8 animate-spin rounded-full border-3 border-zinc-600 border-t-blue-500" />
                        <div className="w-full max-w-50">
                          <div className="mb-1 flex justify-between text-[10px] text-zinc-500">
                            <span>{timeStr} elapsed</span>
                            <span>~{pct}%</span>
                          </div>
                          <div className="h-1.5 w-full overflow-hidden rounded-full bg-zinc-700">
                            <div
                              className="h-full rounded-full bg-blue-500 transition-all duration-1000"
                              style={{ width: `${pct}%` }}
                            />
                          </div>
                        </div>
                        <p className="text-xs text-zinc-500">
                          {chain.length > 0 ? `Generating segment #${chain.length + 1}...` : `Generating #${i + 1}...`}
                        </p>
                      </div>
                    );
                  })()}

                  {video.status === "failed" && (
                    <div
                      style={ratioStyle}
                      className="flex w-full items-center justify-center rounded-lg border border-red-900/50 bg-red-950/20 p-4"
                    >
                      <p className="text-xs text-red-400 text-center">
                        Failed: {video.error}
                      </p>
                    </div>
                  )}

                  {video.status === "completed" && video.video_url && (
                    <video
                      src={video.video_url}
                      autoPlay
                      loop
                      muted
                      playsInline
                      style={ratioStyle}
                      className="w-full rounded-lg bg-black object-contain"
                    />
                  )}

                  <div className="mt-2 flex items-center justify-between">
                    <span className="text-xs text-zinc-500">
                      {chain.length > 0 ? `Segment #${chain.length + 1}` : `Variation #${i + 1}`}
                    </span>
                    {selectedVideo === video.id && (
                      <Badge className="bg-blue-600 text-xs">Selected</Badge>
                    )}
                  </div>
                </Card>
              ))}
            </div>

            {/* Actions for selected/completed video */}
            {selectedVideo && (() => {
              const selVideo = videos.find((v) => v.id === selectedVideo);
              if (!selVideo || selVideo.status !== "completed" || !selVideo.video_url) return null;
              return (
                <div className="mt-6 flex gap-3">
                  <Button asChild className="flex-1" variant="outline">
                    <a href={selVideo.video_url} download>
                      Download Video
                    </a>
                  </Button>
                  <Button
                    className="flex-1 bg-purple-600 hover:bg-purple-700"
                    disabled={isContinuing || isGenerating}
                    onClick={() => startContinuation(selVideo.video_url!, selVideo.id)}
                  >
                    {isContinuing ? "Capturing frame..." : "Continue (Add Segment)"}
                  </Button>
                  {/* Add to chain without continuing */}
                  {chain.length > 0 && !chain.some((s) => s.jobId === selVideo.id) && (
                    <Button
                      variant="outline"
                      onClick={() => addToChain(selVideo)}
                    >
                      Add to Chain
                    </Button>
                  )}
                  <Button variant="outline" onClick={reset}>
                    Start Over
                  </Button>
                </div>
              );
            })()}
          </div>
        )}

        {/* Empty state */}
        {videos.length === 0 && !isGenerating && !showContinuePanel && (
          <Card className="border-zinc-800 bg-zinc-900 p-12 text-center">
            <p className="text-zinc-500">
              Upload a product image and generate video variations to choose
              from
            </p>
          </Card>
        )}
      </main>
    </div>
  );
}
