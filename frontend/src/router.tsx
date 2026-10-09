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
      { path: '*', element: <NotFoundPage /> },
    ],
  },
])
