import React from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import App from './App'
import { ConsoleProvider } from './lib/console'
import './index.css'

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ConsoleProvider>
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </ConsoleProvider>
  </React.StrictMode>,
)
