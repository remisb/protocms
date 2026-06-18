import { useEffect, useRef, useState } from "react";
import { Image as ImageIcon, Loader2, Upload, X } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

interface Props {
  id?: string;
  value: string;
  onChange: (url: string) => void;
}

export function ImageField({ id, value, onChange }: Props) {
  const inputRef = useRef<HTMLInputElement>(null);
  const dropRef = useRef<HTMLDivElement>(null);
  const [uploading, setUploading] = useState(false);
  const [dragOver, setDragOver] = useState(false);
  const [pasteFocused, setPasteFocused] = useState(false);

  const upload = async (file: File | Blob, filename?: string) => {
    setUploading(true);
    try {
      const res = await api.uploadFile(file, filename);
      onChange(res.url);
      toast.success("Image uploaded");
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setUploading(false);
    }
  };

  // Clipboard paste: only intercept while the drop zone has keyboard focus,
  // so it doesn't hijack paste on every field in the dialog.
  useEffect(() => {
    if (!pasteFocused) return;
    const onPaste = (e: ClipboardEvent) => {
      const items = e.clipboardData?.items;
      if (!items) return;
      for (const item of items) {
        if (item.kind === "file" && item.type.startsWith("image/")) {
          const file = item.getAsFile();
          if (file) {
            e.preventDefault();
            void upload(file);
            return;
          }
        }
      }
    };
    window.addEventListener("paste", onPaste);
    return () => window.removeEventListener("paste", onPaste);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pasteFocused]);

  const onDrop = (e: React.DragEvent<HTMLDivElement>) => {
    e.preventDefault();
    setDragOver(false);
    const file = e.dataTransfer.files?.[0];
    if (file && file.type.startsWith("image/")) {
      void upload(file);
    } else if (file) {
      toast.error("Only image files are allowed");
    }
  };

  const clear = () => onChange("");

  return (
    <div className="space-y-2">
      <div
        ref={dropRef}
        tabIndex={0}
        onFocus={() => setPasteFocused(true)}
        onBlur={() => setPasteFocused(false)}
        onDragOver={(e) => {
          e.preventDefault();
          setDragOver(true);
        }}
        onDragLeave={() => setDragOver(false)}
        onDrop={onDrop}
        onClick={() => inputRef.current?.click()}
        className={cn(
          "group relative flex min-h-[140px] cursor-pointer flex-col items-center justify-center gap-2 rounded-md border-2 border-dashed bg-muted/20 p-4 text-center text-sm outline-none transition-colors",
          "hover:bg-muted/40 focus:border-ring focus:bg-muted/40",
          dragOver && "border-primary bg-primary/5",
          uploading && "pointer-events-none opacity-70",
        )}
      >
        {value ? (
          <PreviewBox url={value} onRemove={clear} />
        ) : (
          <>
            {uploading ? (
              <Loader2 className="size-6 animate-spin text-muted-foreground" />
            ) : (
              <Upload className="size-6 text-muted-foreground" />
            )}
            <div className="text-xs text-muted-foreground">
              <span className="font-medium text-foreground">
                Click to upload
              </span>{" "}
              · drag &amp; drop · or focus then paste (⌘V)
            </div>
            <div className="text-[10px] text-muted-foreground">
              PNG, JPG, GIF, WebP, SVG · up to 8 MB
            </div>
          </>
        )}
        <input
          ref={inputRef}
          type="file"
          accept="image/png,image/jpeg,image/gif,image/webp,image/svg+xml"
          className="sr-only"
          onChange={(e) => {
            const file = e.target.files?.[0];
            if (file) void upload(file);
            e.target.value = "";
          }}
        />
      </div>

      <Input
        id={id}
        placeholder="Or paste an image URL"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="text-xs"
      />
    </div>
  );
}

function PreviewBox({ url, onRemove }: { url: string; onRemove: () => void }) {
  const [errored, setErrored] = useState(false);
  useEffect(() => setErrored(false), [url]);

  return (
    <div
      className="flex w-full items-center gap-3"
      onClick={(e) => e.stopPropagation()}
    >
      <div className="flex size-20 shrink-0 items-center justify-center overflow-hidden rounded border bg-background">
        {errored ? (
          <ImageIcon className="size-6 text-muted-foreground" />
        ) : (
          <img
            src={url}
            alt=""
            className="size-full object-contain"
            onError={() => setErrored(true)}
          />
        )}
      </div>
      <div className="min-w-0 flex-1 text-left">
        <a
          href={url}
          target="_blank"
          rel="noreferrer"
          className="block truncate text-xs text-primary underline"
        >
          {url}
        </a>
        <p className="mt-1 text-[10px] text-muted-foreground">
          Click the area above to replace.
        </p>
      </div>
      <Button
        type="button"
        size="icon"
        variant="ghost"
        onClick={(e) => {
          e.stopPropagation();
          onRemove();
        }}
        aria-label="Clear image"
      >
        <X />
      </Button>
    </div>
  );
}
