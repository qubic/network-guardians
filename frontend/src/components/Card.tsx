import { ReactNode } from 'react'

interface CardProps {
  children: ReactNode
  className?: string
}

export default function Card({ children, className = '' }: CardProps) {
  return (
    <div className={`rounded-xl border border-primary-60 bg-primary-70 ${className}`}>
      {children}
    </div>
  )
}
