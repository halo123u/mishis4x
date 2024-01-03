import React from 'react'
import { useContext } from 'react'
import { GlobalDataContext } from '../GlobalDataContext'

const navigation = () => {  
  const { globalData } = useContext(GlobalDataContext)

 return( <header>
    <nav className='row'>
      <a href="/">
        <img src="https://placehold.co/600x200" alt="Company logo" id="logo" />
      </a>
      <div className='flex-space'/>
      {
        globalData && 
        <ul className='row'>
         <li> Hello,{globalData.username}</li>
        <li><button  className='button secondary'>Log out</button></li>
        </ul>
      }
    </nav>
  </header>)
}

export default navigation
