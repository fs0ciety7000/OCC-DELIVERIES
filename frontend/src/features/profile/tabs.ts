export type ProfileTab = 'commandes' | 'infos'

export const PROFILE_TABS: { value: ProfileTab; label: string }[] = [
  { value: 'commandes', label: 'Mes commandes' },
  { value: 'infos', label: 'Mes infos' },
]

export const tabId = (t: ProfileTab) => `profile-tab-${t}`
export const panelId = (t: ProfileTab) => `profile-panel-${t}`
