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
import { AttendancePage } from '../pages/inspection/AttendancePage'
import { AppLayout } from './layouts/AppLayout'
import { ProtectedRoute } from './router/ProtectedRoute'
import { EffectivePermissionRoute } from './router/EffectivePermissionRoute'
import { SuperAdminOnly, SuperAdminRoute } from './router/SuperAdminRoute'

const protectedPage = (page: ReactNode) => <ProtectedRoute>{page}</ProtectedRoute>
const superAdminOnly = (page: ReactNode) => <SuperAdminOnly>{page}</SuperAdminOnly>

export default function App() {
  return <Routes>
    <Route path="/login" element={<LoginPage />} />
    <Route path="/register" element={<RegisterPage />} />
		<Route path="/register/result" element={<RegisterResultPage />} />
		<Route path="/change-password" element={protectedPage(<ChangePasswordPage />)} />
		<Route element={protectedPage(<AppLayout />)}>
			<Route index element={<HomePage />} />
			<Route path="admin" element={<SuperAdminRoute><Outlet /></SuperAdminRoute>}>
				<Route index element={<Navigate to="/" replace />} />
				<Route path="registrations" element={superAdminOnly(<RegistrationReviewPage />)} />
				<Route path="users" element={superAdminOnly(<UserManagementPage />)} />
				<Route path="organization" element={superAdminOnly(<OrganizationConfigPage />)} />
				<Route path="academic" element={superAdminOnly(<AcademicCalendarPage />)} />
				<Route path="master-data" element={<MasterDataPage />} />
				<Route path="dictionaries" element={<DictionaryPage />} />
				<Route path="profile" element={<AccountProfilePage />} />
				<Route path="security" element={<AccountSecurityPage />} />
			</Route>
			<Route path="inspections/attendance" element={<EffectivePermissionRoute action="INSPECTION_VIEW"><AttendancePage /></EffectivePermissionRoute>} />
		</Route>
    <Route path="*" element={<Navigate to="/" replace />} />
  </Routes>
}
