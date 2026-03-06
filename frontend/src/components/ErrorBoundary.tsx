import { Component, ReactNode } from 'react'
import Card from './Card'

interface Props {
  children: ReactNode
}

interface State {
  hasError: boolean
  error: Error | null
}

export default class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props)
    this.state = { hasError: false, error: null }
  }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error }
  }

  handleReload = () => {
    window.location.reload()
  }

  handleGoHome = () => {
    window.location.href = '/'
  }

  render() {
    if (this.state.hasError) {
      return (
        <div className="flex min-h-screen items-center justify-center bg-primary-80 p-4">
          <Card className="max-w-lg p-8 text-center">
            <div className="mb-4 text-5xl">⚠️</div>
            <h1 className="mb-2 font-space text-xl font-semibold text-white">
              Something went wrong
            </h1>
            <p className="mb-6 text-gray-50">
              An unexpected error occurred. Please try reloading the page.
            </p>
            <div className="flex justify-center gap-3">
              <button
                onClick={this.handleReload}
                className="rounded-lg bg-primary-50 px-6 py-2.5 text-sm font-medium text-white transition-colors hover:bg-primary-40"
              >
                Reload Page
              </button>
              <button
                onClick={this.handleGoHome}
                className="rounded-lg bg-primary-60 px-6 py-2.5 text-sm font-medium text-gray-50 transition-colors hover:text-white"
              >
                Go to Dashboard
              </button>
            </div>
          </Card>
        </div>
      )
    }

    return this.props.children
  }
}
