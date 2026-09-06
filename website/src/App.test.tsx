import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import App from './App'

describe('App', () => {
  it('renders the home page heading', () => {
    render(<App />)
    expect(screen.getByRole('heading', { level: 1, name: 'gdxsv' })).toBeInTheDocument()
  })
})
