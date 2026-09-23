/** Locale-aware presentation only; never use these strings in persisted protocols. */
export function formatters(locale:'en'|'es') {
  const language=locale==='es'?'es-AR':'en-US'
  const number=new Intl.NumberFormat(language)
  const relative=new Intl.RelativeTimeFormat(language,{numeric:'auto'})
  const date=new Intl.DateTimeFormat(language,{dateStyle:'short',timeStyle:'medium'})
  return {
    number:(value:number)=>number.format(value),
    money:(value:number,digits=4)=>new Intl.NumberFormat(language,{style:'currency',currency:'USD',minimumFractionDigits:digits,maximumFractionDigits:digits}).format(value),
    date:(value:string)=>Number.isFinite(Date.parse(value))?date.format(new Date(value)):'—',
    relative:(value:string,now=Date.now())=>{
      const milliseconds=Date.parse(value)-now
      if(!Number.isFinite(milliseconds))return '—'
      const minutes=Math.trunc(milliseconds/60000)
      return Math.abs(minutes)<60?relative.format(minutes,'minute'):Math.abs(minutes)<1440?relative.format(Math.trunc(minutes/60),'hour'):relative.format(Math.trunc(minutes/1440),'day')
    },
  }
}
