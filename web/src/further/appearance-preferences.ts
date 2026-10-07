// Use the same controller before paint in classic shells and React documents.
import '../../../further-ui/assets/theme-preference.js'

export type WorkspacePaletteMode = 'fixed' | 'auto'
export type WorkspacePalette = 'white' | 'blue' | 'yellow' | 'duanwu' | 'qixi' | 'mid_autumn' | 'chongyang' | 'new_year_eve' | 'national_day'
export type WorkspacePalettePreference = {
  mode: WorkspacePaletteMode
  fixedPalette: WorkspacePalette
  effectivePalette: WorkspacePalette
  palette: WorkspacePalette | 'auto'
  season: string
  holiday: string
}
type WorkspaceAppearanceRuntime = {
  version: 2
  sync: () => void
  select: (value: string) => void
  setMode: (mode: WorkspacePaletteMode) => void
  applySeason: (season: string) => void
  state: () => WorkspacePalettePreference
  current: () => WorkspacePalette | 'auto'
  mode: () => WorkspacePaletteMode
  season: () => string
}
declare global {
  interface Window {
    FurtherAppearance?: WorkspaceAppearanceRuntime
    FurtherInitializeAppearance?: () => WorkspaceAppearanceRuntime
  }
}
function runtime(): WorkspaceAppearanceRuntime {
  if (window.FurtherAppearance?.version === 2) return window.FurtherAppearance
  const appearance = window.FurtherInitializeAppearance?.()
  if (!appearance) throw new Error('配色设置暂时无法载入，请重新打开页面。')
  return appearance
}
export function initializeWorkspacePalette(): void { runtime().sync() }
export function getWorkspacePalettePreference(): WorkspacePalettePreference { return runtime().state() }
export function selectWorkspacePalette(value: string): void { runtime().select(value) }
export function setWorkspacePaletteMode(mode: WorkspacePaletteMode): void { runtime().setMode(mode) }
export function applyWorkspaceSeason(season: string): void { runtime().applySeason(season) }
