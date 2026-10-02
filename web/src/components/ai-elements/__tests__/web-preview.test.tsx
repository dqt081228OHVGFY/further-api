import { cleanup, render } from '@testing-library/react'
import { afterEach, expect, it } from 'vitest'

import { WebPreview, WebPreviewBody } from '../web-preview'

afterEach(cleanup)

it('keeps preview scripts and forms interactive while isolating the preview origin', () => {
  const { container } = render(<WebPreview defaultUrl='https://preview.example'><WebPreviewBody /></WebPreview>)
  const frame = container.querySelector('iframe') as HTMLIFrameElement
  const permissions = frame.getAttribute('sandbox')?.split(' ')
  expect(permissions).toContain('allow-scripts')
  expect(permissions).toContain('allow-forms')
  expect(permissions).not.toContain('allow-same-origin')
  expect(frame).toHaveAttribute('src', 'https://preview.example')
})
