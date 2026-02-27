"use client";

import { useState, useRef, useEffect } from "react";
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

const MODELS = [
  { id: "google:3@3", name: "Veo 3.1 Fast", price: 0.80, estSeconds: 120 },
  { id: "pixverse:1@7", name: "PixVerse v5.6", price: 0.24, estSeconds: 90 },
  { id: "vidu:4@2", name: "Vidu Q3 Turbo", price: 0.13, estSeconds: 60 },
  { id: "vidu:4@1", name: "Vidu Q3", price: 0.05, estSeconds: 60 },
];

const RATIOS = [
  { value: "9:16", label: "Mobile", desc: "9:16 · TikTok, Reels, Stories" },
  { value: "16:9", label: "Desktop", desc: "16:9 · YouTube, Website" },
  { value: "1:1", label: "Square", desc: "1:1 · Instagram Feed" },
];

type UploadedImage = {
  file: File;
  previewUrl: string;
  filename: string | null;
};

type Scene = {
  id: string;
  prompt: string;
  modelId: string;
  jobId: string | null;
  status: "idle" | "generating" | "completed" | "failed";
  videoUrl: string | null;
  error: string | null;
  isAutoPrompting: boolean;
};

type Project = {
  id: string;
  name: string;
  createdAt: string;
};

function generateId() {
  return Math.random().toString(36).slice(2, 10);
}

// ─── Project List View ───────────────────────────────────────────────
function ProjectListView({
  projects,
  onCreateProject,
  onSelectProject,
  onDeleteProject,
}: {
  projects: Project[];
  onCreateProject: (name: string) => void;
  onSelectProject: (id: string) => void;
  onDeleteProject: (id: string) => void;
}) {
  const [newName, setNewName] = useState("");

  return (
    <div className="min-h-screen bg-zinc-950 text-white">
      <header className="border-b border-zinc-800 px-6 py-4">
        <div className="mx-auto flex max-w-4xl items-center justify-between">
          <h1 className="text-xl font-bold">Product Video AI</h1>
          <Badge variant="secondary">Demo</Badge>
        </div>
      </header>

      <main className="mx-auto max-w-4xl px-6 py-10">
        <div className="mb-8 text-center">
          <h2 className="text-3xl font-bold">Your Projects</h2>
          <p className="mt-2 text-zinc-400">
            Each project is a product ad — upload images, build scenes, generate videos.
          </p>
        </div>

        {/* Create project */}
        <Card className="mb-6 border-zinc-800 bg-zinc-900 p-4">
          <div className="flex gap-3">
            <input
              type="text"
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && newName.trim()) {
                  onCreateProject(newName.trim());
                  setNewName("");
                }
              }}
              placeholder="New project name (e.g. Wireless Earbuds Ad)"
              className="flex-1 rounded-lg border border-zinc-700 bg-zinc-800 px-3 py-2 text-sm text-white placeholder-zinc-500 focus:border-blue-500 focus:outline-none"
            />
            <Button
              onClick={() => {
                if (newName.trim()) {
                  onCreateProject(newName.trim());
                  setNewName("");
                }
              }}
              disabled={!newName.trim()}
              className="bg-blue-600 hover:bg-blue-700"
            >
              Create Project
            </Button>
          </div>
        </Card>

        {/* Project list */}
        {projects.length === 0 ? (
          <Card className="border-zinc-800 bg-zinc-900 p-12 text-center">
            <p className="text-zinc-500">No projects yet. Create one above to get started.</p>
          </Card>
        ) : (
          <div className="grid gap-3">
            {projects.map((project) => (
              <Card
                key={project.id}
                className="border-zinc-800 bg-zinc-900 p-4 transition-all hover:border-zinc-600 cursor-pointer"
                onClick={() => onSelectProject(project.id)}
              >
                <div className="flex items-center justify-between">
                  <div>
                    <h3 className="font-semibold text-white">{project.name}</h3>
                    <p className="text-xs text-zinc-500">{project.createdAt}</p>
                  </div>
                  <div className="flex items-center gap-2">
                    <button
                      onClick={(e) => {
                        e.stopPropagation();
                        onDeleteProject(project.id);
                      }}
                      className="rounded-md px-2 py-1 text-xs text-zinc-500 hover:bg-zinc-800 hover:text-red-400"
                    >
                      Delete
                    </button>
                    <svg className="h-4 w-4 text-zinc-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
                    </svg>
                  </div>
                </div>
              </Card>
            ))}
          </div>
        )}
      </main>
    </div>
  );
}

// ─── Asset Setup Modal ───────────────────────────────────────────────
function AssetModal({
  images,
  productName,
  ratio,
  isUploading,
  onAddFiles,
  onRemoveImage,
  onUpload,
  onSetProductName,
  onSetRatio,
  onSave,
  onCancel,
  canCancel,
}: {
  images: UploadedImage[];
  productName: string;
  ratio: string;
  isUploading: boolean;
  onAddFiles: (files: FileList | File[]) => void;
  onRemoveImage: (index: number) => void;
  onUpload: () => void;
  onSetProductName: (name: string) => void;
  onSetRatio: (ratio: string) => void;
  onSave: () => void;
  onCancel: () => void;
  canCancel: boolean;
}) {
  const [dragActive, setDragActive] = useState(false);
  const allUploaded = images.length > 0 && images.every((img) => img.filename !== null);

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm">
      <div className="mx-4 w-full max-w-lg rounded-xl border border-zinc-700 bg-zinc-900 p-6 shadow-2xl">
        <div className="mb-5 flex items-center justify-between">
          <h2 className="text-lg font-bold text-white">Project Setup</h2>
          {canCancel && (
            <button onClick={onCancel} className="text-zinc-500 hover:text-white">
              <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          )}
        </div>

        {/* Product name */}
        <div className="mb-4">
          <p className="mb-1 text-xs font-medium text-zinc-400">Product name</p>
          <input
            type="text"
            value={productName}
            onChange={(e) => onSetProductName(e.target.value)}
            placeholder="e.g. wireless earbuds, sunglasses, sneakers..."
            className="w-full rounded-lg border border-zinc-700 bg-zinc-800 px-3 py-2 text-sm text-white placeholder-zinc-500 focus:border-blue-500 focus:outline-none"
          />
        </div>

        {/* Image upload */}
        <div className="mb-4">
          <p className="mb-1 text-xs font-medium text-zinc-400">
            Product images <span className="font-normal text-zinc-500">(up to 2)</span>
          </p>
          <div className="grid grid-cols-2 gap-2">
            {images.map((img, i) => (
              <div key={i} className="relative group">
                <img
                  src={img.previewUrl}
                  alt={`Image ${i + 1}`}
                  className="h-28 w-full rounded-lg object-contain bg-zinc-800"
                />
                <button
                  onClick={() => onRemoveImage(i)}
                  className="absolute right-1 top-1 rounded-full bg-black/70 p-0.5 text-white opacity-0 transition-opacity group-hover:opacity-100 hover:bg-black"
                >
                  <svg className="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                  </svg>
                </button>
                <span className="absolute bottom-1 left-1 rounded bg-black/70 px-1 py-0.5 text-[9px] text-zinc-300">
                  {i === 0 ? "First" : "Last"}
                </span>
                {img.filename && (
                  <span className="absolute bottom-1 right-1 rounded bg-green-600/80 px-1 py-0.5 text-[9px] text-white">
                    OK
                  </span>
                )}
              </div>
            ))}

            {images.length < 2 && (
              <div
                onDrop={(e) => {
                  e.preventDefault();
                  setDragActive(false);
                  if (e.dataTransfer.files?.length) onAddFiles(e.dataTransfer.files);
                }}
                onDragOver={(e) => { e.preventDefault(); setDragActive(true); }}
                onDragLeave={() => setDragActive(false)}
                className={`flex h-28 cursor-pointer flex-col items-center justify-center rounded-lg border-2 border-dashed transition-colors ${
                  dragActive
                    ? "border-blue-500 bg-blue-500/10"
                    : "border-zinc-700 hover:border-zinc-500"
                }`}
                onClick={() => document.getElementById("modal-file-input")?.click()}
              >
                <svg className="mb-1 h-5 w-5 text-zinc-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M12 4v16m8-8H4" />
                </svg>
                <p className="text-xs text-zinc-500">
                  {images.length === 0 ? "Add image" : "Add second angle"}
                </p>
                <input
                  id="modal-file-input"
                  type="file"
                  accept="image/jpeg,image/png,image/webp"
                  multiple
                  className="hidden"
                  onChange={(e) => {
                    if (e.target.files?.length) onAddFiles(e.target.files);
                    e.target.value = "";
                  }}
                />
              </div>
            )}
          </div>
        </div>

        {/* Aspect ratio */}
        <div className="mb-5">
          <p className="mb-1 text-xs font-medium text-zinc-400">Aspect ratio</p>
          <div className="grid grid-cols-3 gap-2">
            {RATIOS.map((r) => (
              <button
                key={r.value}
                onClick={() => onSetRatio(r.value)}
                className={`rounded-lg border px-2 py-1.5 text-left text-xs transition-colors ${
                  ratio === r.value
                    ? "border-blue-500 bg-blue-500/10 text-white"
                    : "border-zinc-700 bg-zinc-800 text-zinc-400 hover:border-zinc-500"
                }`}
              >
                <span className="font-medium">{r.label}</span>
                <span className="block text-[10px] text-zinc-500">{r.value}</span>
              </button>
            ))}
          </div>
        </div>

        {/* Actions */}
        <div className="flex gap-3">
          {images.length > 0 && !allUploaded ? (
            <Button className="flex-1 bg-blue-600 hover:bg-blue-700" onClick={onUpload} disabled={isUploading}>
              {isUploading
                ? `Uploading... (${images.filter((i) => i.filename).length}/${images.length})`
                : `Upload & Save`}
            </Button>
          ) : (
            <Button
              className="flex-1 bg-blue-600 hover:bg-blue-700"
              onClick={onSave}
              disabled={!allUploaded || !productName.trim()}
            >
              Save
            </Button>
          )}
          {canCancel && (
            <Button variant="outline" onClick={onCancel}>
              Cancel
            </Button>
          )}
        </div>

        {!productName.trim() && images.length > 0 && (
          <p className="mt-2 text-[10px] text-yellow-500/70">Enter a product name to continue</p>
        )}
      </div>
    </div>
  );
}

// ─── Project Detail View ─────────────────────────────────────────────
function ProjectDetailView({
  project,
  onBack,
}: {
  project: Project;
  onBack: () => void;
}) {
  // Assets (managed via modal)
  const [images, setImages] = useState<UploadedImage[]>([]);
  const [productName, setProductName] = useState("");
  const [isUploading, setIsUploading] = useState(false);
  const [showModal, setShowModal] = useState(true); // open on first entry
  const [ratio, setRatio] = useState("9:16");

  // Scenes
  const [scenes, setScenes] = useState<Scene[]>([
    { id: generateId(), prompt: "", modelId: "vidu:4@1", jobId: null, status: "idle", videoUrl: null, error: null, isAutoPrompting: false },
  ]);

  // Polling & timer
  const pollingRef = useRef<Set<string>>(new Set());
  const [elapsed, setElapsed] = useState<Record<string, number>>({});
  const timerRef = useRef<Record<string, number>>({});

  const allUploaded = images.length > 0 && images.every((img) => img.filename !== null);
  const uploadedFilenames = images.filter((img) => img.filename).map((img) => img.filename!);
  const isAnyGenerating = scenes.some((s) => s.status === "generating");

  // Timer effect for generating scenes
  useEffect(() => {
    const interval = setInterval(() => {
      const now = Date.now();
      setElapsed((prev) => {
        const updated = { ...prev };
        scenes.forEach((scene) => {
          if (scene.status === "generating" && timerRef.current[scene.id]) {
            updated[scene.id] = Math.floor((now - timerRef.current[scene.id]) / 1000);
          }
        });
        return updated;
      });
    }, 1000);
    return () => clearInterval(interval);
  }, [scenes]);

  // Image helpers
  const addFiles = (files: FileList | File[]) => {
    const newImages: UploadedImage[] = [];
    for (const file of Array.from(files)) {
      if (!file.type.startsWith("image/")) continue;
      if (images.length + newImages.length >= 2) break;
      newImages.push({ file, previewUrl: URL.createObjectURL(file), filename: null });
    }
    if (newImages.length > 0) {
      setImages((prev) => [...prev, ...newImages]);
    }
  };

  const removeImage = (index: number) => {
    setImages((prev) => {
      const copy = [...prev];
      URL.revokeObjectURL(copy[index].previewUrl);
      copy.splice(index, 1);
      return copy;
    });
  };

  const uploadImages = async () => {
    const pending = images.filter((img) => img.filename === null);
    if (pending.length === 0) return;
    setIsUploading(true);
    try {
      for (let i = 0; i < images.length; i++) {
        if (images[i].filename !== null) continue;
        const formData = new FormData();
        formData.append("image", images[i].file);
        const res = await fetch(`${API_URL}/api/upload`, { method: "POST", body: formData });
        const data = await res.json();
        if (data.error) throw new Error(data.error);
        setImages((prev) => {
          const copy = [...prev];
          copy[i] = { ...copy[i], filename: data.filename };
          return copy;
        });
      }
      // Auto-close modal after successful upload
      setShowModal(false);
    } catch (err) {
      alert("Upload failed: " + (err as Error).message);
    } finally {
      setIsUploading(false);
    }
  };

  // Scene operations
  const addScene = () => {
    setScenes((prev) => [
      ...prev,
      { id: generateId(), prompt: "", modelId: "vidu:4@1", jobId: null, status: "idle", videoUrl: null, error: null, isAutoPrompting: false },
    ]);
  };

  const removeScene = (id: string) => {
    if (scenes.length <= 1) return;
    setScenes((prev) => prev.filter((s) => s.id !== id));
  };

  const updateScene = (id: string, updates: Partial<Scene>) => {
    setScenes((prev) => prev.map((s) => (s.id === id ? { ...s, ...updates } : s)));
  };

  const autoPromptScene = async (sceneId: string) => {
    if (uploadedFilenames.length === 0) return;
    const sceneIndex = scenes.findIndex((s) => s.id === sceneId);
    if (sceneIndex === -1) return;

    updateScene(sceneId, { isAutoPrompting: true });
    try {
      const res = await fetch(`${API_URL}/api/auto-prompt`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          filenames: uploadedFilenames,
          product_name: productName,
          scene_number: sceneIndex + 1,
          total_scenes: scenes.length,
        }),
      });
      const data = await res.json();
      if (data.error) throw new Error(data.error);
      updateScene(sceneId, { prompt: data.prompt, isAutoPrompting: false });
    } catch (err) {
      alert("Auto prompt failed: " + (err as Error).message);
      updateScene(sceneId, { isAutoPrompting: false });
    }
  };

  const pollScene = async (sceneId: string, jobId: string) => {
    pollingRef.current.add(sceneId);
    while (pollingRef.current.has(sceneId)) {
      await new Promise((r) => setTimeout(r, 3000));
      try {
        const res = await fetch(`${API_URL}/api/status/${jobId}`);
        const data = await res.json();
        if (data.status === "completed") {
          pollingRef.current.delete(sceneId);
          delete timerRef.current[sceneId];
          updateScene(sceneId, { status: "completed", videoUrl: data.video_url });
          return;
        }
        if (data.status === "failed") {
          pollingRef.current.delete(sceneId);
          delete timerRef.current[sceneId];
          updateScene(sceneId, { status: "failed", error: data.error });
          return;
        }
      } catch {
        // ignore polling errors
      }
    }
  };

  const generateScene = async (sceneId: string) => {
    const scene = scenes.find((s) => s.id === sceneId);
    if (!scene || !allUploaded) return;

    updateScene(sceneId, { status: "generating", videoUrl: null, error: null, jobId: null });
    timerRef.current[sceneId] = Date.now();

    try {
      const res = await fetch(`${API_URL}/api/generate`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          filenames: uploadedFilenames,
          prompt: scene.prompt,
          model: scene.modelId,
          ratio: ratio,
          product_name: productName,
        }),
      });
      const data = await res.json();
      if (data.error) throw new Error(data.error);

      updateScene(sceneId, { jobId: data.job_id });
      pollScene(sceneId, data.job_id);
    } catch (err) {
      delete timerRef.current[sceneId];
      updateScene(sceneId, { status: "failed", error: (err as Error).message });
    }
  };

  const generateAll = async () => {
    if (!allUploaded) return;
    for (const scene of scenes) {
      if (scene.prompt.trim() && scene.status !== "generating") {
        generateScene(scene.id);
      }
    }
  };

  const totalCost = scenes.reduce((sum, s) => {
    const model = MODELS.find((m) => m.id === s.modelId);
    return sum + (model?.price || 0);
  }, 0);

  const completedCount = scenes.filter((s) => s.status === "completed").length;

  return (
    <div className="min-h-screen bg-zinc-950 text-white">
      {/* Asset setup modal */}
      {showModal && (
        <AssetModal
          images={images}
          productName={productName}
          ratio={ratio}
          isUploading={isUploading}
          onAddFiles={addFiles}
          onRemoveImage={removeImage}
          onUpload={uploadImages}
          onSetProductName={setProductName}
          onSetRatio={setRatio}
          onSave={() => setShowModal(false)}
          onCancel={() => setShowModal(false)}
          canCancel={allUploaded && productName.trim() !== ""}
        />
      )}

      <header className="border-b border-zinc-800 px-6 py-4">
        <div className="mx-auto flex max-w-6xl items-center justify-between">
          <div className="flex items-center gap-3">
            <button onClick={onBack} className="text-zinc-400 hover:text-white">
              <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
              </svg>
            </button>
            <h1 className="text-xl font-bold">{project.name}</h1>
          </div>
          <div className="flex items-center gap-3">
            {/* Edit assets button */}
            <button
              onClick={() => setShowModal(true)}
              className="flex items-center gap-1.5 rounded-md border border-zinc-700 bg-zinc-800 px-2.5 py-1 text-xs text-zinc-400 hover:border-zinc-500 hover:text-white"
            >
              {allUploaded && images.length > 0 && (
                <div className="flex -space-x-1">
                  {images.map((img, i) => (
                    <img key={i} src={img.previewUrl} alt="" className="h-4 w-4 rounded-sm border border-zinc-700 object-cover" />
                  ))}
                </div>
              )}
              <span>{allUploaded ? "Edit Assets" : "Setup Assets"}</span>
            </button>
            {completedCount > 0 && (
              <Badge variant="secondary" className="bg-green-600/20 text-green-300">
                {completedCount}/{scenes.length} done
              </Badge>
            )}
            <Badge variant="secondary">Demo</Badge>
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-6xl px-6 py-6">
        {/* Asset preview bar */}
        {allUploaded && (
          <div className="mb-4 flex items-center gap-3 rounded-lg border border-zinc-800 bg-zinc-900/50 px-3 py-2">
            <div className="flex gap-2">
              {images.map((img, i) => (
                <img key={i} src={img.previewUrl} alt="" className="h-10 w-10 rounded-md border border-zinc-700 object-contain bg-zinc-800" />
              ))}
            </div>
            <div className="text-xs">
              <span className="font-medium text-white">{productName}</span>
              <span className="ml-2 text-zinc-500">{RATIOS.find((r) => r.value === ratio)?.desc}</span>
            </div>
          </div>
        )}

        {/* Warning if no assets */}
        {!allUploaded && !showModal && (
          <Card className="mb-6 border-yellow-500/30 bg-yellow-950/10 p-4">
            <p className="text-sm text-yellow-300">
              Set up your product images and name first.
            </p>
            <Button size="sm" variant="outline" className="mt-2" onClick={() => setShowModal(true)}>
              Setup Assets
            </Button>
          </Card>
        )}

        {/* Generate All + Add Scene controls */}
        <div className="mb-4 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <Button
              onClick={generateAll}
              disabled={!allUploaded || isAnyGenerating || scenes.every((s) => !s.prompt.trim())}
              className="bg-blue-600 hover:bg-blue-700"
            >
              {isAnyGenerating
                ? "Generating..."
                : `Generate All (${scenes.filter((s) => s.prompt.trim()).length} scene${scenes.filter((s) => s.prompt.trim()).length !== 1 ? "s" : ""} · $${totalCost.toFixed(2)})`}
            </Button>
            <span className="text-xs text-zinc-500">
              Each scene = 4s video
            </span>
          </div>
          <Button variant="outline" size="sm" onClick={addScene}>
            + Add Scene
          </Button>
        </div>

        {/* Scene list */}
        <div className="space-y-4">
          {scenes.map((scene, index) => {
            const model = MODELS.find((m) => m.id === scene.modelId)!;
            const sceneElapsed = elapsed[scene.id] || 0;
            const pct = scene.status === "generating"
              ? Math.min(Math.round((sceneElapsed / model.estSeconds) * 100), 95)
              : 0;

            return (
              <Card key={scene.id} className="border-zinc-800 bg-zinc-900 p-4">
                <div className="flex items-start gap-4">
                  {/* Scene number */}
                  <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-zinc-800 text-sm font-bold text-zinc-400">
                    {index + 1}
                  </div>

                  {/* Scene content */}
                  <div className="flex-1 min-w-0">
                    <div className="mb-3 flex items-center gap-2">
                      <h4 className="text-sm font-semibold text-white">Scene {index + 1}</h4>
                      {scene.status === "completed" && (
                        <Badge className="bg-green-600/20 text-green-300 text-[10px]">Done</Badge>
                      )}
                      {scene.status === "failed" && (
                        <Badge className="bg-red-600/20 text-red-300 text-[10px]">Failed</Badge>
                      )}
                      {scenes.length > 1 && (
                        <button
                          onClick={() => removeScene(scene.id)}
                          className="ml-auto text-xs text-zinc-600 hover:text-red-400"
                        >
                          Remove
                        </button>
                      )}
                    </div>

                    {/* Prompt + Auto Prompt */}
                    <div className="mb-3">
                      <div className="mb-1 flex items-center justify-between">
                        <p className="text-xs text-zinc-400">Prompt</p>
                        <button
                          onClick={() => autoPromptScene(scene.id)}
                          disabled={!allUploaded || scene.isAutoPrompting}
                          className="rounded-md bg-purple-600 px-2 py-0.5 text-[10px] font-medium text-white transition-colors hover:bg-purple-700 disabled:opacity-40 disabled:cursor-not-allowed"
                        >
                          {scene.isAutoPrompting ? "Analyzing..." : "Auto Prompt"}
                        </button>
                      </div>
                      <textarea
                        value={scene.prompt}
                        onChange={(e) => {
                          updateScene(scene.id, { prompt: e.target.value });
                          e.target.style.height = "auto";
                          e.target.style.height = e.target.scrollHeight + "px";
                        }}
                        ref={(el) => {
                          if (el) {
                            el.style.height = "auto";
                            el.style.height = el.scrollHeight + "px";
                          }
                        }}
                        placeholder="Describe this scene... or click Auto Prompt"
                        className="w-full resize-none overflow-hidden rounded-lg border border-zinc-700 bg-zinc-800 p-2 text-xs text-white placeholder-zinc-600 focus:border-blue-500 focus:outline-none"
                        rows={2}
                      />
                    </div>

                    {/* Model selector + Generate */}
                    <div className="flex items-center gap-3">
                      <div className="w-48">
                        <Select
                          value={scene.modelId}
                          onValueChange={(v) => updateScene(scene.id, { modelId: v })}
                        >
                          <SelectTrigger className="h-8 border-zinc-700 bg-zinc-800 text-xs text-white">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent className="border-zinc-700 bg-zinc-800">
                            {MODELS.map((m) => (
                              <SelectItem key={m.id} value={m.id} className="text-xs text-white hover:bg-zinc-700">
                                {m.name} · ${m.price.toFixed(2)}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      </div>

                      <Button
                        size="sm"
                        onClick={() => generateScene(scene.id)}
                        disabled={!allUploaded || !scene.prompt.trim() || scene.status === "generating"}
                        className="bg-blue-600 hover:bg-blue-700 text-xs"
                      >
                        {scene.status === "generating" ? "Generating..." : `Generate ($${model.price.toFixed(2)})`}
                      </Button>
                    </div>

                    {/* Error */}
                    {scene.status === "failed" && scene.error && (
                      <p className="mt-2 text-xs text-red-400">Error: {scene.error}</p>
                    )}

                    {/* Progress */}
                    {scene.status === "generating" && (
                      <div className="mt-3">
                        <div className="flex justify-between text-[10px] text-zinc-500 mb-1">
                          <span>{sceneElapsed}s elapsed</span>
                          <span>~{pct}%</span>
                        </div>
                        <div className="h-1.5 w-full overflow-hidden rounded-full bg-zinc-700">
                          <div
                            className="h-full rounded-full bg-blue-500 transition-all duration-1000"
                            style={{ width: `${pct}%` }}
                          />
                        </div>
                      </div>
                    )}

                    {/* Video result */}
                    {scene.status === "completed" && scene.videoUrl && (
                      <div className="mt-3">
                        <video
                          src={scene.videoUrl}
                          autoPlay
                          loop
                          muted
                          playsInline
                          className="w-full max-w-sm rounded-lg bg-black object-contain"
                          style={{ aspectRatio: ratio.replace(":", " / ") }}
                        />
                        <div className="mt-2 flex gap-2">
                          <Button asChild size="sm" variant="outline" className="text-xs">
                            <a href={scene.videoUrl} download>
                              Download
                            </a>
                          </Button>
                          <Button
                            size="sm"
                            variant="outline"
                            className="text-xs"
                            onClick={() => {
                              updateScene(scene.id, { status: "idle", videoUrl: null, error: null, jobId: null });
                            }}
                          >
                            Re-generate
                          </Button>
                        </div>
                      </div>
                    )}
                  </div>
                </div>
              </Card>
            );
          })}
        </div>

        {/* Add scene button at bottom */}
        <button
          onClick={addScene}
          className="mt-4 flex w-full items-center justify-center gap-2 rounded-lg border-2 border-dashed border-zinc-800 py-3 text-sm text-zinc-500 transition-colors hover:border-zinc-600 hover:text-zinc-300"
        >
          <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M12 4v16m8-8H4" />
          </svg>
          Add Scene
        </button>
      </main>
    </div>
  );
}

// ─── Main App ────────────────────────────────────────────────────────
export default function Home() {
  const [projects, setProjects] = useState<Project[]>([]);
  const [activeProjectId, setActiveProjectId] = useState<string | null>(null);

  const createProject = (name: string) => {
    const project: Project = {
      id: generateId(),
      name,
      createdAt: new Date().toLocaleDateString(),
    };
    setProjects((prev) => [project, ...prev]);
    setActiveProjectId(project.id);
  };

  const deleteProject = (id: string) => {
    setProjects((prev) => prev.filter((p) => p.id !== id));
    if (activeProjectId === id) setActiveProjectId(null);
  };

  const activeProject = projects.find((p) => p.id === activeProjectId);

  if (activeProject) {
    return (
      <ProjectDetailView
        project={activeProject}
        onBack={() => setActiveProjectId(null)}
      />
    );
  }

  return (
    <ProjectListView
      projects={projects}
      onCreateProject={createProject}
      onSelectProject={setActiveProjectId}
      onDeleteProject={deleteProject}
    />
  );
}
