import '@fontsource-variable/inter'
import '@fontsource-variable/bricolage-grotesque'
import './styles/index.css'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { refreshAuth } from '@/lib/auth'
import { registerServiceWorker } from '@/pwa/register'
import { App } from './App'

void refreshAuth()
registerServiceWorker()

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
