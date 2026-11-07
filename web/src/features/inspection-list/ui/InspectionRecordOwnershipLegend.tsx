export function InspectionRecordOwnershipLegend() {
  return (
    <span className="inspection-record-ownership-legend" role="note" aria-label="表格行底色表示记录录入人">
      <span className="inspection-record-ownership-legend-item">
        <span className="inspection-record-ownership-swatch inspection-record-ownership-swatch--mine" aria-hidden="true" />
        本人录入
      </span>
      <span className="inspection-record-ownership-legend-item">
        <span className="inspection-record-ownership-swatch inspection-record-ownership-swatch--other" aria-hidden="true" />
        他人录入
      </span>
    </span>
  )
}
