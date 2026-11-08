import type { ReactNode } from 'react'
import { Navigate, Outlet, Route, Routes } from 'react-router-dom'
import { AcademicCalendarPage } from '../pages/admin/AcademicCalendarPage'
import { AccountProfilePage } from '../pages/admin/AccountProfilePage'
import { AccountSecurityPage } from '../pages/admin/AccountSecurityPage'
import { OrganizationConfigPage } from '../pages/admin/OrganizationConfigPage'
import { RegistrationReviewPage } from '../pages/admin/RegistrationReviewPage'
import { UserManagementPage } from '../pages/admin/UserManagementPage'
import { MasterDataPage } from '../pages/admin/MasterDataPage'
import { DictionaryPage } from '../pages/admin/DictionaryPage'
import { ChangePasswordPage } from '../pages/auth/ChangePasswordPage'
import { LoginPage } from '../pages/auth/LoginPage'
import { RegisterPage } from '../pages/auth/RegisterPage'
import { RegisterResultPage } from '../pages/auth/RegisterResultPage'
import { HomePage } from '../pages/home/HomePage'
import { LateReturnPage } from '../pages/inspection/LateReturnPage'
import { EveningSelfStudyPage } from '../pages/inspection/EveningSelfStudyPage'
import { MorningAttendancePage } from '../pages/inspection/MorningAttendancePage'
import { ClassroomHygienePage } from '../pages/inspection/ClassroomHygienePage'
import { PublicDormitoryHygienePage } from '../pages/inspection/PublicDormitoryHygienePage'
import { DormitoryHygienePage } from '../pages/inspection/DormitoryHygienePage'
import { UnattendedClassroomPage } from '../pages/inspection/UnattendedClassroomPage'
import { InspectionTypeRoute } from './router/InspectionTypeRoute'
import { AttendancePage } from '../pages/inspection/AttendancePage'
import { WeeklyReportSummaryPage } from '../pages/report/WeeklyReportSummaryPage'
import { WeeklyReportArchivePage } from '../pages/report/WeeklyReportArchivePage'
import { ReportOutputSettingsPage } from '../pages/report/ReportOutputSettingsPage'
import { AppLayout } from './layouts/AppLayout'
import { ProtectedRoute } from './router/ProtectedRoute'
import { SuperAdminOnly, SuperAdminRoute } from './router/SuperAdminRoute'
import { EffectivePermissionRoute } from './router/EffectivePermissionRoute'

const protectedPage = (page: ReactNode) => <ProtectedRoute>{page}</ProtectedRoute>
const superAdminOnly = (page: ReactNode) => <SuperAdminOnly>{page}</SuperAdminOnly>
const adminOrSystemAdmin = (page: ReactNode) => <SuperAdminRoute>{page}</SuperAdminRoute>

export default function App() {
  return <Routes>
    <Route path="/login" element={<LoginPage />} />
    <Route path="/register" element={<RegisterPage />} />
		<Route path="/register/result" element={<RegisterResultPage />} />
		<Route path="/change-password" element={protectedPage(<ChangePasswordPage />)} />
		<Route element={protectedPage(<AppLayout />)}>
			<Route index element={<HomePage />} />
			<Route path="admin" element={<Outlet />}>
				<Route index element={adminOrSystemAdmin(<Navigate to="/" replace />)} />
				<Route path="registrations" element={superAdminOnly(<RegistrationReviewPage />)} />
				<Route path="users" element={superAdminOnly(<UserManagementPage />)} />
				<Route path="organization" element={superAdminOnly(<OrganizationConfigPage />)} />
				<Route path="academic" element={<EffectivePermissionRoute action="MORNING_SCHEDULE_MANAGE"><AcademicCalendarPage /></EffectivePermissionRoute>} />
				<Route path="master-data" element={adminOrSystemAdmin(<MasterDataPage />)} />
				<Route path="dictionaries" element={adminOrSystemAdmin(<DictionaryPage />)} />
				<Route path="profile" element={adminOrSystemAdmin(<AccountProfilePage />)} />
				<Route path="security" element={adminOrSystemAdmin(<AccountSecurityPage />)} />
			</Route>
			<Route path="inspections/attendance" element={<InspectionTypeRoute typeCode="ATTENDANCE_RATE"><AttendancePage /></InspectionTypeRoute>} />
			<Route path="reports/workbench" element={<EffectivePermissionRoute action="REPORT_VIEW"><WeeklyReportSummaryPage /></EffectivePermissionRoute>} />
			<Route path="reports/weekly-summary" element={<Navigate to="/reports/workbench" replace />} />
			<Route path="reports/weekly-draft" element={<Navigate to="/reports/workbench" replace />} />
			<Route path="reports/archive" element={<EffectivePermissionRoute action="ARCHIVE_VIEW"><WeeklyReportArchivePage /></EffectivePermissionRoute>} />
					<Route path="reports/output-settings" element={<EffectivePermissionRoute action="REPORT_OUTPUT_SETTINGS_MANAGE"><ReportOutputSettingsPage /></EffectivePermissionRoute>} />
			<Route path="inspections/late-return" element={<InspectionTypeRoute typeCode="LATE_RETURN"><LateReturnPage /></InspectionTypeRoute>} />
			<Route path="inspections/evening-self-study" element={<InspectionTypeRoute typeCode="EVENING_SELF_STUDY"><EveningSelfStudyPage /></InspectionTypeRoute>} />
			<Route path="inspections/morning-attendance" element={<InspectionTypeRoute typeCode="MORNING_ATTENDANCE"><MorningAttendancePage /></InspectionTypeRoute>} />
			<Route path="inspections/public-dormitory-hygiene" element={<InspectionTypeRoute typeCode="PUBLIC_DORMITORY_HYGIENE"><PublicDormitoryHygienePage /></InspectionTypeRoute>} />
			<Route path="inspections/classroom-hygiene" element={<InspectionTypeRoute typeCode="CLASSROOM_HYGIENE"><ClassroomHygienePage /></InspectionTypeRoute>} />
			<Route path="inspections/dormitory-hygiene" element={<InspectionTypeRoute typeCode="DORMITORY_HYGIENE"><DormitoryHygienePage /></InspectionTypeRoute>} />
			<Route path="inspections/unattended-classroom" element={<InspectionTypeRoute typeCode="UNATTENDED_CLASSROOM"><UnattendedClassroomPage /></InspectionTypeRoute>} />
		</Route>
    <Route path="*" element={<Navigate to="/" replace />} />
  </Routes>
}
