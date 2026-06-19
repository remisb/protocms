import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { Toaster } from "sonner";
import { AppLayout } from "@/components/app-layout";
import { AuthProvider, useAuth } from "@/lib/auth";
import { DesignerPage } from "@/pages/designer";
import { EditorPage } from "@/pages/editor";
import { LoginPage } from "@/pages/login";
import "./index.css";

function App() {
  const { status } = useAuth();

  if (status === "loading") {
    return (
      <div className="flex min-h-screen items-center justify-center text-sm text-muted-foreground">
        connecting…
      </div>
    );
  }

  if (status === "anon") {
    return <LoginPage />;
  }

  return (
    <Routes>
      <Route path="/" element={<AppLayout />}>
        <Route index element={<Navigate to="/designer" replace />} />
        <Route path="designer" element={<DesignerPage />} />
        <Route path="editor" element={<EditorPage />} />
        <Route path="editor/:contentType" element={<EditorPage />} />
      </Route>
    </Routes>
  );
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <BrowserRouter>
      <AuthProvider>
        <App />
      </AuthProvider>
    </BrowserRouter>
    <Toaster richColors closeButton position="top-right" />
  </StrictMode>,
);
