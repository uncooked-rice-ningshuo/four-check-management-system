import type { CSSProperties, ReactNode } from "react";
import {
  AUTH_REGISTER_FORM_SPACING,
  AUTH_SHELL_DESKTOP_LAYOUT,
} from "./authLayoutConfig";
import { BrandLockup } from "./BrandLockup";

export function AuthLayout({
  variant,
  children,
}: {
  variant: "login" | "register";
  children: ReactNode;
}) {
  const shellVariables = {
    "--auth-shell-height": AUTH_SHELL_DESKTOP_LAYOUT.height,
    "--auth-shell-columns": AUTH_SHELL_DESKTOP_LAYOUT.columns,
    ...(variant === "register"
      ? {
          "--auth-register-item-gap": AUTH_REGISTER_FORM_SPACING.itemGap,
          "--auth-register-label-gap": AUTH_REGISTER_FORM_SPACING.labelGap,
        }
      : {}),
  } as CSSProperties;

  return (
    <main className="auth-page">
      <section className={`auth-shell ${variant}`} style={shellVariables}>
        <aside className="auth-visual">
          <BrandLockup className="system-brand--auth" />
          {variant === "login" && (
            <div className="visual-copy">
              <h1>
                把每一周的日常管理，
                <br />
                留成可追溯的数据。
              </h1>
              <p>日常检查 · 数据确认 · 通报制作 · 历史归档</p>
            </div>
          )}
        </aside>
        <section className="auth-panel">{children}</section>
      </section>
    </main>
  );
}
