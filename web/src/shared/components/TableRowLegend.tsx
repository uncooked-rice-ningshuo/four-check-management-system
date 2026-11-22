export function TableRowLegend({ label, primary, secondary }: { label: string; primary: string; secondary: string }) {
  return (
    <span className="inspection-record-ownership-legend" role="note" aria-label={label}>
      <span className="inspection-record-ownership-legend-item">
        <span className="inspection-record-ownership-swatch inspection-record-ownership-swatch--mine" aria-hidden="true" />
        {primary}
      </span>
      <span className="inspection-record-ownership-legend-item">
        <span className="inspection-record-ownership-swatch inspection-record-ownership-swatch--other" aria-hidden="true" />
        {secondary}
      </span>
    </span>
  )
}
