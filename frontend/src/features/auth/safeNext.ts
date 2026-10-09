/** N'accepte que des chemins internes (évite les redirections ouvertes). */
export function safeNext(next: string | null): string {
  return next && next.startsWith('/') && !next.startsWith('//') ? next : '/'
}
