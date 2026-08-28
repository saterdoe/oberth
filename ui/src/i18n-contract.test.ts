import ts from 'typescript'
import {expect,it} from 'vitest'

const sources=import.meta.glob<string>(['./App.tsx','./ProductTour.tsx','./Modal.tsx'],{query:'?raw',import:'default',eager:true})

it('requires explicit catalog calls for visible JSX copy',()=>{
  const exceptions=new Set(['Oberth','Ollama','OpenAI','Anthropic','Google','Qdrant','Claude Code','Codex CLI','OpenCode CLI','Antigravity','LM Studio','API key','Base URL','JSON','QA','Git','tokens','auto','localhost:9090','Alt','Ctrl/⌘','K','Esc','v','oberth v'])
  const violations:string[]=[]
  for(const file of ['App.tsx','ProductTour.tsx','Modal.tsx']){
    const source=sources[`./${file}`]
    const ast=ts.createSourceFile(file,source,ts.ScriptTarget.Latest,true,ts.ScriptKind.TSX)
    const visit=(node:ts.Node)=>{
      let text=''
      if(ts.isJsxText(node))text=node.text.trim()
      if(ts.isJsxAttribute(node)&&['aria-label','placeholder','label','text','detail','title','alt'].includes(node.name.getText(ast))&&node.initializer&&ts.isStringLiteral(node.initializer))text=node.initializer.text
      if(/\p{L}/u.test(text)&&!exceptions.has(text))violations.push(`${file}:${ast.getLineAndCharacterOfPosition(node.getStart(ast)).line+1}: ${text}`)
      ts.forEachChild(node,visit)
    }
    visit(ast)
    expect(source).not.toContain('CompatLocalization')
    expect(source).not.toContain('MutationObserver')
  }
  expect(violations).toEqual([])
})
