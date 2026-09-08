import * as React from "react";
import { RotateCcw, TriangleAlert } from "lucide-react";
import { useOptionalI18n } from "../i18n";
import { Button } from "./ui/button";

/**
 * Contains a render failure to the panel that caused it. Without this, one broken
 * component unmounts the whole React tree and the clinic is left with a blank screen
 * mid-consultation. `resetKey` clears the error when the caller navigates elsewhere.
 */
export class ErrorBoundary extends React.Component<
  { children: React.ReactNode; label?: string; resetKey?: unknown },
  { error: Error | null }
> {
  state: { error: Error | null } = { error: null };

  static getDerivedStateFromError(error: Error) {
    return { error };
  }

  componentDidUpdate(previous: { resetKey?: unknown }) {
    if (this.state.error && previous.resetKey !== this.props.resetKey) this.setState({ error: null });
  }

  componentDidCatch(error: Error, info: React.ErrorInfo) {
    console.error(`[SentryMed] ${this.props.label ?? "panel"} failed to render`, error, info.componentStack);
  }

  render() {
    if (!this.state.error) return this.props.children;
    return <PanelFailure label={this.props.label} error={this.state.error} onRetry={() => this.setState({ error: null })} />;
  }
}

function PanelFailure({ label, error, onRetry }: { label?: string; error: Error; onRetry(): void }) {
  const { t } = useOptionalI18n();
  return (
    <div role="alert" className="grid gap-3 rounded-lg border border-amber-300 bg-amber-50 p-4 text-sm text-amber-900">
      <div className="flex items-center gap-2 font-semibold">
        <TriangleAlert className="h-4 w-4 shrink-0" />
        {label ? `${t("This section could not be displayed")} — ${label}` : t("This section could not be displayed")}
      </div>
      <p>{t("The rest of the application is still usable. Nothing was lost or saved incorrectly.")}</p>
      <details className="text-xs opacity-80">
        <summary className="cursor-pointer font-semibold">{t("Technical detail")}</summary>
        <pre className="mt-2 max-h-40 overflow-auto whitespace-pre-wrap break-words">{error.message}</pre>
      </details>
      <div>
        <Button size="sm" variant="outline" onClick={onRetry}>
          <RotateCcw className="h-3.5 w-3.5" />
          {t("Try this section again")}
        </Button>
      </div>
    </div>
  );
}
