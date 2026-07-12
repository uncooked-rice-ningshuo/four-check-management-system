import { Navigate, Route, Routes } from 'react-router-dom'
import { LoginPage } from '../pages/auth/LoginPage'
import { RegisterPage } from '../pages/auth/RegisterPage'
import { RegisterResultPage } from '../pages/auth/RegisterResultPage'
import { HomePage } from '../pages/home/HomePage'
import { ProtectedRoute } from './router/ProtectedRoute'

export default function App() {
  return <Routes><Route path="/login" element={<LoginPage />} /><Route path="/register" element={<RegisterPage />} /><Route path="/register/result" element={<RegisterResultPage />} /><Route path="/" element={<ProtectedRoute><HomePage /></ProtectedRoute>} /><Route path="*" element={<Navigate to="/" replace />} /></Routes>
}
