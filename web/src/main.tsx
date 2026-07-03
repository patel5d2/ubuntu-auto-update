import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App.tsx'
// Bundled (not CDN): the backend's CSP style-src blocks third-party hosts,
// so a CDN stylesheet never loads in the production container.
import '@picocss/pico/css/pico.min.css'
import './shell.css'

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
)
