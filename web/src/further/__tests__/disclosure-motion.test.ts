import {afterEach,describe,expect,it,vi} from 'vitest'
import {mountDisclosureMotion} from '../disclosure-motion'

afterEach(()=>vi.restoreAllMocks())
function fixture(){
  const details=document.createElement('details')
  details.innerHTML='<summary>请求说明</summary><p>完整内容</p>'
  document.body.append(details)
  details.getBoundingClientRect=()=>({height:details.open?220:44,width:300,left:0,right:300,top:0,bottom:details.open?220:44,x:0,y:0,toJSON(){}})
  const effects:Array<{resolve:()=>void;cancel:ReturnType<typeof vi.fn>}>=[]
  const animate=vi.fn(()=>{
    let resolve!:()=>void
    const finished=new Promise<void>(done=>{resolve=done})
    const cancel=vi.fn()
    effects.push({resolve,cancel})
    return {finished,cancel} as unknown as Animation
  })
  details.animate=animate
  return {details,summary:details.querySelector('summary')!,effects,animate,dispose:mountDisclosureMotion(document)}
}
describe('disclosure expansion motion',()=>{
  it('animates expansion and closing without rewriting the existing content',async()=>{
    const {details,summary,effects,animate,dispose}=fixture()
    summary.click()
    expect(animate).toHaveBeenLastCalledWith([{height:'44px'},{height:'220px'}],expect.objectContaining({duration:220}))
    effects[0].resolve();await Promise.resolve();await Promise.resolve()
    expect(details.open).toBe(true)
    summary.click()
    expect(details.open).toBe(true)
    expect(details.dataset.disclosureState).toBe('closing')
    expect(animate).toHaveBeenLastCalledWith([{height:'220px'},{height:'44px'}],expect.objectContaining({duration:140}))
    expect(details.querySelector('p')).toHaveAttribute('inert')
    effects[1].resolve();await Promise.resolve();await Promise.resolve()
    expect(details.open).toBe(false)
    expect(details.querySelector('p')?.parentElement).toBe(details)
    expect(details.style.overflow).toBe('')
    expect(details.querySelector('p')).not.toHaveAttribute('inert')
    dispose();details.remove()
  })
  it('cancels an interrupted expansion and restores state on feature disposal',()=>{
    const {details,summary,effects,dispose}=fixture()
    summary.click();summary.click()
    expect(effects[0].cancel).toHaveBeenCalledOnce()
    dispose()
    expect(details.open).toBe(false)
    expect(details.style.overflow).toBe('')
    expect(details.dataset.disclosureMotionEngine).toBeUndefined()
    details.remove()
  })
})
