import '@fontsource-variable/inter'
import '@fontsource-variable/bricolage-grotesque'
import './styles/index.css'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { refreshAuth } from '@/lib/auth'
import { App } from './App'

void refreshAuth()

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
