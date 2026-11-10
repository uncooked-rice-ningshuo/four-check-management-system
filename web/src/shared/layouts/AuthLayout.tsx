import type { CSSProperties, ReactNode } from "react";
import {
  AUTH_REGISTER_FORM_SPACING,
  AUTH_SHELL_DESKTOP_LAYOUT,
} from "./authLayoutConfig";

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
        <section className="auth-panel">
          <img className="auth-combination-logo" src="/assets/brand/combination-logo.png" alt="湖南科技大学计算机科学与工程学院学生日常管理系统" />
          {children}
        </section>
        <aside className="auth-visual" aria-hidden="true" />
      </section>
    </main>
  );
}
