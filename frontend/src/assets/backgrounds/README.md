# Campus background artwork

`particles.js` adapts the particle tree and water scene from the HTML supplied by
the project owner on 2026-09-22 (Anchor AI reference). Its bundled Three.js code
retains the MIT attribution. The reference's marketing UI, logos, remote images,
custom cursor, loader and scrolling page are omitted; no insurance brand appears
in the Campus UI. `mountScene(canvas)` provides pause/resume/dispose lifecycle
methods, caps pixel density at 1.25 and renders at approximately 30 fps.

The three video sources are preserved verbatim in `src/brand/scenes.ts`.
They are loaded only for the active route. Playback pauses when the document is
hidden, the user pauses backgrounds, or reduced motion is requested. Failure to
load external media leaves a readable dark fallback.

Self-hosted deployments with a custom CSP must permit the exact video origin
`https://d8j0ntlcm91z4.cloudfront.net` in `media-src`. The default server policy
includes it; no script origin or other permission is expanded.
