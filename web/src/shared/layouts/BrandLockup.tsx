type BrandLockupProps = {
  className?: string;
};

export function BrandLockup({ className = "" }: BrandLockupProps) {
  return (
    <div className={`system-brand ${className}`.trim()}>
      <span className="system-brand-mark" aria-hidden="true">
        <img src="/logo.png" alt="" />
      </span>
      <span className="system-brand-copy">
        <strong className="system-brand-acronym">HNUST</strong>
        <span className="system-brand-university">湖南科技大学</span>
        <span className="system-brand-college">计算机科学与工程学院</span>
        <span className="system-brand-purpose">
          <span className="system-brand-ai">AI</span>
          <span>学生日常工作</span>
        </span>
      </span>
    </div>
  );
}
