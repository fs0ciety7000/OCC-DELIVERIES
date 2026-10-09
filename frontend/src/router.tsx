import { lazy } from 'react'
import { createBrowserRouter } from 'react-router'
import { AppShell } from '@/components/layout/AppShell'
import { RequireAuth } from '@/components/layout/RequireAuth'
import { NotFoundPage, RouteError } from '@/components/layout/RouteError'
import { LoginPage } from '@/features/auth/LoginPage'
import { RegisterPage } from '@/features/auth/RegisterPage'
import { HomePage } from '@/features/home/HomePage'

// Écrans secondaires chargés à la demande.
const RestaurantsPage = lazy(() => import('@/features/restaurants/RestaurantsPage').then((m) => ({ default: m.RestaurantsPage })))
const RestaurantDetailPage = lazy(() => import('@/features/restaurants/RestaurantDetailPage').then((m) => ({ default: m.RestaurantDetailPage })))
const PartyPage = lazy(() => import('@/features/party/PartyPage').then((m) => ({ default: m.PartyPage })))
const JoinPage = lazy(() => import('@/features/party/JoinPage').then((m) => ({ default: m.JoinPage })))
const AdminLayout = lazy(() => import('@/features/admin/AdminLayout').then((m) => ({ default: m.AdminLayout })))
const DashboardPage = lazy(() => import('@/features/admin/DashboardPage').then((m) => ({ default: m.DashboardPage })))
const RestaurantsAdminPage = lazy(() => import('@/features/admin/RestaurantsAdminPage').then((m) => ({ default: m.RestaurantsAdminPage })))
const MenuEditorPage = lazy(() => import('@/features/admin/MenuEditorPage').then((m) => ({ default: m.MenuEditorPage })))
const ImportPage = lazy(() => import('@/features/admin/ImportPage').then((m) => ({ default: m.ImportPage })))
const PartiesAdminPage = lazy(() => import('@/features/admin/PartiesAdminPage').then((m) => ({ default: m.PartiesAdminPage })))
const PartyAdminDetailPage = lazy(() => import('@/features/admin/PartiesAdminPage').then((m) => ({ default: m.PartyAdminDetailPage })))
const SyncPage = lazy(() => import('@/features/admin/SyncPage').then((m) => ({ default: m.SyncPage })))
const UsersAdminPage = lazy(() => import('@/features/admin/UsersAdminPage').then((m) => ({ default: m.UsersAdminPage })))
const ProfilePage = lazy(() => import('@/features/profile/ProfilePage').then((m) => ({ default: m.ProfilePage })))

export const router = createBrowserRouter([
  {
    element: <AppShell />,
    errorElement: <RouteError />,
    children: [
      { path: '/', element: <HomePage /> },
      { path: '/login', element: <LoginPage /> },
      { path: '/register', element: <RegisterPage /> },
      { path: '/restaurants', element: <RestaurantsPage /> },
      { path: '/restaurants/:id', element: <RestaurantDetailPage /> },
      {
        path: '/j/:code',
        element: (
          <RequireAuth>
            <JoinPage />
          </RequireAuth>
        ),
      },
      {
        path: '/party/:id',
        element: (
          <RequireAuth>
            <PartyPage />
          </RequireAuth>
        ),
      },
      {
        path: '/profile',
        element: (
          <RequireAuth>
            <ProfilePage />
          </RequireAuth>
        ),
      },
      {
        path: '/admin',
        element: <AdminLayout />,
        children: [
          { index: true, element: <DashboardPage /> },
          { path: 'restaurants', element: <RestaurantsAdminPage /> },
          { path: 'restaurants/:id', element: <MenuEditorPage /> },
          { path: 'import', element: <ImportPage /> },
          { path: 'synchronisation', element: <SyncPage /> },
          { path: 'commandes', element: <PartiesAdminPage /> },
          { path: 'commandes/:id', element: <PartyAdminDetailPage /> },
          { path: 'utilisateurs', element: <UsersAdminPage /> },
        ],
      },
      { path: '*', element: <NotFoundPage /> },
    ],
  },
])
