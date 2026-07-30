import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App'
import './app.css'
import { bootstrapRuntimeConfig, retryRuntimeStartup } from './runtimeConfig'
import {I18nProvider,savedLocale,translate} from './i18n'

async function start() {
  const root = ReactDOM.createRoot(document.getElementById('root')!)
  const t=(key:Parameters<typeof translate>[1])=>translate(savedLocale(),key)
  const renderPreparing = (message=t('startup.preparing')) => root.render(
    <main className="startup-screen" role="status" aria-live="polite">
      <img src="/oberth-wordmark.svg" alt="Oberth"/>
      <span className="startup-spinner" aria-hidden="true"/>
      <strong>{message}</strong>
      <p>{t('startup.detail')}</p>
    </main>,
  )
  const renderError = (message:string) => root.render(
    <main className="startup-screen startup-error" role="alert">
      <img src="/oberth-wordmark.svg" alt="Oberth"/>
      <strong>{t('startup.failed')}</strong>
      <p>{message}</p>
      <button onClick={async()=>{renderPreparing(t('startup.retrying'));const next=await retryRuntimeStartup();if(next.state==='ready')renderApp();else renderError(next.error||t('startup.noResponse'))}}>{t('common.retry')}</button>
    </main>,
  )
  const renderApp = () => root.render(<React.StrictMode><I18nProvider><App /></I18nProvider></React.StrictMode>)

  renderPreparing()
  let config = await bootstrapRuntimeConfig()
  if (!config.desktop) {
    renderApp()
    return
  }
  while (config.state === 'preparing') {
    await new Promise(resolve=>window.setTimeout(resolve,200))
    config = await bootstrapRuntimeConfig()
  }
  if (config.state === 'ready') renderApp()
  else renderError(config.error||t('startup.noResponse'))
}

void start()
