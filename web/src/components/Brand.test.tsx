import { render, screen } from '@testing-library/react'
import { ThemeProvider } from '@mui/material'
import { describe, expect, it } from 'vitest'
import { Brand } from './Brand'
import { theme } from '../theme'

describe('Brand', () => {
  it('renders the product identity', () => {
    render(<ThemeProvider theme={theme}><Brand /></ThemeProvider>)
    expect(screen.getByText('ORBIT')).toBeInTheDocument()
    expect(screen.getByText('RELATIONSHIPS')).toBeInTheDocument()
  })
})

