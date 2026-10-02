import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { PromptInput, PromptInputAttachments, PromptInputTextarea } from '../prompt-input'

beforeEach(() => {
  Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: vi.fn(() => 'blob:attached-file') })
  Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: vi.fn() })
})

afterEach(() => { cleanup(); vi.unstubAllGlobals() })

describe('prompt attachment errors', () => {
  it('reports a failed blob conversion and keeps the text and attachment available for retry', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('unreadable attachment')))
    const submit = vi.fn()
    const onError = vi.fn()
    render(<PromptInput onSubmit={submit} onError={onError}><PromptInputTextarea aria-label='Message' /><PromptInputAttachments>{(file) => <span>{file.filename}</span>}</PromptInputAttachments><button type='submit'>Send</button></PromptInput>)
    fireEvent.change(screen.getByLabelText('Upload files'), { target: { files: [new File(['text'], 'notes.txt', { type: 'text/plain' })] } })
    fireEvent.change(screen.getByLabelText('Message'), { target: { value: 'Please read my notes' } })

    fireEvent.click(screen.getByRole('button', { name: 'Send' }))

    await waitFor(() => { expect(onError).toHaveBeenCalledWith(expect.objectContaining({ code: 'file_conversion' })) })
    expect(submit).not.toHaveBeenCalled()
    expect(screen.getByLabelText('Message')).toHaveValue('Please read my notes')
    expect(screen.getByText('notes.txt')).toBeInTheDocument()
  })

  it('restores the local draft after an asynchronous submission rejects', async () => {
    const submit = vi.fn().mockRejectedValue(new Error('request failed'))
    render(<PromptInput onSubmit={submit}><PromptInputTextarea aria-label='Message' /><button type='submit'>Send</button></PromptInput>)
    fireEvent.change(screen.getByLabelText('Message'), { target: { value: 'Keep this draft' } })
    fireEvent.click(screen.getByRole('button', { name: 'Send' }))
    await waitFor(() => { expect(screen.getByLabelText('Message')).toHaveValue('Keep this draft') })
    expect(submit).toHaveBeenCalledWith(expect.objectContaining({ text: 'Keep this draft' }), expect.anything())
  })

  it('preserves text typed while a previous attachment conversion is pending', async () => {
    let rejectFetch: (reason: Error) => void = () => {}
    vi.stubGlobal('fetch', vi.fn(() => new Promise((_resolve, reject) => { rejectFetch = reject })))
    const onError = vi.fn()
    render(<PromptInput onSubmit={vi.fn()} onError={onError}><PromptInputTextarea aria-label='Message' /><button type='submit'>Send</button></PromptInput>)
    fireEvent.change(screen.getByLabelText('Upload files'), { target: { files: [new File(['text'], 'notes.txt', { type: 'text/plain' })] } })
    fireEvent.change(screen.getByLabelText('Message'), { target: { value: 'Original draft' } })
    fireEvent.click(screen.getByRole('button', { name: 'Send' }))
    fireEvent.change(screen.getByLabelText('Message'), { target: { value: 'My next message' } })
    rejectFetch(new Error('attachment unavailable'))
    await waitFor(() => { expect(onError).toHaveBeenCalled() })
    expect(screen.getByLabelText('Message')).toHaveValue('My next message')
  })
})
