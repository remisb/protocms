import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { Toaster } from "sonner";
import { AppLayout } from "@/components/app-layout";
import { DesignerPage } from "@/pages/designer";
import { EditorPage } from "@/pages/editor";
import "./index.css";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<AppLayout />}>
          <Route index element={<Navigate to="/designer" replace />} />
          <Route path="designer" element={<DesignerPage />} />
          <Route path="editor" element={<EditorPage />} />
          <Route path="editor/:contentType" element={<EditorPage />} />
        </Route>
      </Routes>
    </BrowserRouter>
    <Toaster richColors closeButton position="top-right" />
  </StrictMode>,
);
