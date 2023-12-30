import React from 'react'
import { useContext } from 'react'
import { AuthContext } from '../AuthContext'

const navigation = () => {  
  const { user } = useContext(AuthContext)

 return( <header>
    <nav>
      <a href="/">
        <img src="https://placehold.co/600x200" alt="Company logo" id="logo" />
      </a>
      <ul>{ !!user ? <li>{user.username}</li> : null }</ul>
    </nav>
  </header>)
}

export default navigation
