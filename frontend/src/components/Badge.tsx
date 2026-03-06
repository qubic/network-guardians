import { ReactNode } from 'react'

type BadgeVariant = 'success' | 'error' | 'warning' | 'info' | 'default'

interface BadgeProps {
  children: ReactNode
  variant?: BadgeVariant
}

const variantStyles: Record<BadgeVariant, string> = {
  success: 'bg-success-90 text-success-40',
  error: 'bg-error-90 text-error-40',
  warning: 'bg-warning-90 text-warning-40',
  info: 'bg-primary-60 text-primary-30',
  default: 'bg-primary-60 text-gray-50'
}

export default function Badge({ children, variant = 'default' }: BadgeProps) {
  return (
    <span
      className={`inline-flex items-center rounded-md px-2 py-1 text-xs font-medium ${variantStyles[variant]}`}
    >
      {children}
    </span>
  )
}
