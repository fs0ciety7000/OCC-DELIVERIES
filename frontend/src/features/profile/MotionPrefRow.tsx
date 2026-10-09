import { Toggle } from '@/features/admin/Toggle'
import { useMotionPref } from '@/lib/motionPref'

/** Profil → Apparence : « Animations réduites » (s'ajoute à `prefers-reduced-motion`). */
export function MotionPrefRow() {
  const { userReduced, systemReduced, setUserReduced } = useMotionPref()
  return (
    <div className="flex items-start justify-between gap-4 border-t border-border pt-3">
      <div className="min-w-0">
        <p id="motion-pref-label" className="font-medium">
          Animations réduites
        </p>
        <p className="text-sm text-muted">
          Coupe les animations décoratives (scooter, confettis, plats qui volent) et les vibrations.
          {systemReduced && ' Ton appareil le demande déjà : elles sont coupées.'}
        </p>
      </div>
      <Toggle checked={userReduced || systemReduced} disabled={systemReduced} onChange={setUserReduced} label="Animations réduites" />
    </div>
  )
}
