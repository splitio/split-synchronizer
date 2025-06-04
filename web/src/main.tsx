import React from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App.tsx'

// import '@blueprintjs/core/lib/css/blueprint.css';
// import '@blueprintjs/datetime/lib/css/blueprint-datetime.css';
// import '@harness/uicore/dist/index.css';

const container = document.getElementById('root')
if (!container) throw new Error('Root container missing in index.html')

const root = createRoot(container)

root.render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
)