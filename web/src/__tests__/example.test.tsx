import { render, screen } from '@testing-library/react'

describe('Example Test', () => {
  it('should pass', () => {
    expect(true).toBe(true)
  })

  it('should render a component', () => {
    const TestComponent = () => <div>Hello Test</div>
    render(<TestComponent />)
    expect(screen.getByText('Hello Test')).toBeInTheDocument()
  })

  it('should fail to demonstrate error handling', () => {
    expect(1 + 1).toBe(3) // This will fail
  })
})